package monitoring

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemMonitor surveille les métriques système (réseau, CPU, RAM)
type SystemMonitor struct {
	state       *types.ServerState
	vmClient    *victoria.MetricsClient
	process     *process.Process
	networkIfs  []string
	lastNetIO   map[string]psnet.IOCountersStat
	lastCPUTime time.Time
	diskPaths   []string
}

// NewSystemMonitor crée un nouveau moniteur système
func NewSystemMonitor(state *types.ServerState, vmClient *victoria.MetricsClient) (*SystemMonitor, error) {
	// Obtenir le PID du processus actuel
	pid := os.Getpid()
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil, fmt.Errorf("failed to get process info: %w", err)
	}

	// Obtenir les interfaces réseau
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Filtrer les interfaces valides
	var validIfs []string
	for _, iface := range interfaces {
		// Ignorer les interfaces loopback et down
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			validIfs = append(validIfs, iface.Name)
		}
	}

	// Obtenir les chemins de disque
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk partitions: %w", err)
	}

	var diskPaths []string
	for _, part := range partitions {
		// Ignorer les systèmes de fichiers spéciaux
		if !strings.HasPrefix(part.Fstype, "dev") && !strings.HasPrefix(part.Fstype, "proc") {
			diskPaths = append(diskPaths, part.Mountpoint)
		}
	}

	// Initialiser le moniteur
	sm := &SystemMonitor{
		state:       state,
		vmClient:    vmClient,
		process:     proc,
		networkIfs:  validIfs,
		lastNetIO:   make(map[string]psnet.IOCountersStat),
		lastCPUTime: time.Now(),
		diskPaths:   diskPaths,
	}

	// Initialiser les statistiques réseau
	netStats, err := psnet.IOCounters(true)
	if err == nil {
		for _, stat := range netStats {
			for _, ifName := range validIfs {
				if stat.Name == ifName {
					sm.lastNetIO[ifName] = stat
					break
				}
			}
		}
	}

	return sm, nil
}

// Start démarre la collecte des métriques système
func (sm *SystemMonitor) Start(ctx context.Context) {
	// Collecter les métriques toutes les 15 secondes
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Collecter immédiatement au démarrage
	sm.collectNetworkMetrics()
	sm.collectCPUMetrics()
	sm.collectMemoryMetrics()
	sm.collectDiskMetrics()
	sm.collectGoMetrics()
	sm.collectServerState()

	// Boucle principale de collecte
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.collectNetworkMetrics()
			sm.collectCPUMetrics()
			sm.collectMemoryMetrics()
			sm.collectDiskMetrics()
			sm.collectGoMetrics()
			sm.collectServerState()
		}
	}
}

// collectNetworkMetrics collecte les métriques réseau
func (sm *SystemMonitor) collectNetworkMetrics() {
	// Obtenir les statistiques réseau
	netStats, err := psnet.IOCounters(true)
	if err != nil {
		utils.LogError("Failed to get network stats: %v", err)
		return
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{},
		Time:    time.Now(),
	}

	// Traiter chaque interface
	for _, stat := range netStats {
		// Vérifier si c'est une interface que nous surveillons
		isMonitored := false
		for _, ifName := range sm.networkIfs {
			if stat.Name == ifName {
				isMonitored = true
				break
			}
		}

		if !isMonitored {
			continue
		}

		// Créer les labels pour cette interface
		labels := map[string]string{
			"server_id":   sm.state.ServerID,
			"server_name": sm.state.ServerName,
			"server_type": sm.state.ServerType,
			"interface":   stat.Name,
		}

		// Calculer les deltas si nous avons des données précédentes
		if lastStat, ok := sm.lastNetIO[stat.Name]; ok {
			// Temps écoulé depuis la dernière mesure
			elapsed := time.Since(sm.lastCPUTime).Seconds()
			if elapsed <= 0 {
				elapsed = 1 // Éviter la division par zéro
			}

			// Calculer les taux par seconde
			bytesSentRate := float64(stat.BytesSent-lastStat.BytesSent) / elapsed
			bytesRecvRate := float64(stat.BytesRecv-lastStat.BytesRecv) / elapsed
			packetsSentRate := float64(stat.PacketsSent-lastStat.PacketsSent) / elapsed
			packetsRecvRate := float64(stat.PacketsRecv-lastStat.PacketsRecv) / elapsed
			errorsRate := float64(stat.Errin+stat.Errout-lastStat.Errin-lastStat.Errout) / elapsed
			dropsRate := float64(stat.Dropin+stat.Dropout-lastStat.Dropin-lastStat.Dropout) / elapsed

			// Ajouter les métriques
			batch.Metrics = append(batch.Metrics, []types.Metric{
				{
					Name:        metrics.NetworkBytesSent.Name,
					Value:       bytesSentRate,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: labels,
				},
				{
					Name:        metrics.NetworkBytesReceived.Name,
					Value:       bytesRecvRate,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: labels,
				},
				{
					Name:        metrics.NetworkPacketsSent.Name,
					Value:       packetsSentRate,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: labels,
				},
				{
					Name:        metrics.NetworkPacketsReceived.Name,
					Value:       packetsRecvRate,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: labels,
				},
				{
					Name:        metrics.NetworkErrors.Name,
					Value:       errorsRate,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: labels,
				},
				{
					Name:        metrics.NetworkDrops.Name,
					Value:       dropsRate,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: labels,
				},
			}...)
		}

		// Mettre à jour les statistiques précédentes
		sm.lastNetIO[stat.Name] = stat
	}

	// Mettre à jour le temps de la dernière mesure
	sm.lastCPUTime = time.Now()

	// Envoyer les métriques
	if len(batch.Metrics) > 0 {
		if err := sm.vmClient.SendMetrics(batch); err != nil {
			utils.LogError("Failed to send network metrics: %v", err)
		}
	}
}

// collectCPUMetrics collecte les métriques CPU
func (sm *SystemMonitor) collectCPUMetrics() {
	// Obtenir l'utilisation CPU du processus
	procCPU, err := sm.process.CPUPercent()
	if err != nil {
		utils.LogError("Failed to get process CPU usage: %v", err)
		return
	}

	// Obtenir l'utilisation CPU du système
	sysCPU, err := cpu.Percent(0, true)
	if err != nil {
		utils.LogError("Failed to get system CPU usage: %v", err)
		return
	}

	// Créer les labels de base
	baseLabels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerCPUUsage.Name,
				Value:       procCPU,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Ajouter les métriques CPU par cœur
	for i, usage := range sysCPU {
		cpuLabels := cloneLabels(baseLabels)
		cpuLabels["cpu"] = fmt.Sprintf("cpu%d", i)
		batch.Metrics = append(batch.Metrics, types.Metric{
			Name:        metrics.SystemCPUUsage.Name,
			Value:       usage,
			Type:        types.Gauge,
			Timestamp:   time.Now(),
			LabelValues: cpuLabels,
		})
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Failed to send CPU metrics: %v", err)
	}
}

// collectMemoryMetrics collecte les métriques mémoire
func (sm *SystemMonitor) collectMemoryMetrics() {
	// Obtenir l'utilisation mémoire du processus
	procMem, err := sm.process.MemoryInfo()
	if err != nil {
		utils.LogError("Failed to get process memory usage: %v", err)
		return
	}

	// Obtenir l'utilisation mémoire du système
	sysMem, err := mem.VirtualMemory()
	if err != nil {
		utils.LogError("Failed to get system memory usage: %v", err)
		return
	}

	// Obtenir l'utilisation swap
	sysSwap, err := mem.SwapMemory()
	if err != nil {
		utils.LogError("Failed to get swap memory usage: %v", err)
		return
	}

	// Créer les labels de base
	baseLabels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerMemoryUsage.Name,
				Value:       float64(procMem.RSS),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemoryTotal.Name,
				Value:       float64(sysMem.Total),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemoryUsed.Name,
				Value:       float64(sysMem.Used),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemoryFree.Name,
				Value:       float64(sysMem.Free),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemorySwap.Name,
				Value:       float64(sysSwap.Used),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Failed to send memory metrics: %v", err)
	}
}

// collectDiskMetrics collecte les métriques disque
func (sm *SystemMonitor) collectDiskMetrics() {
	// Créer les labels de base
	baseLabels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{},
		Time:    time.Now(),
	}

	// Collecter les métriques pour chaque chemin de disque
	for _, path := range sm.diskPaths {
		usage, err := disk.Usage(path)
		if err != nil {
			utils.LogError("Failed to get disk usage for %s: %v", path, err)
			continue
		}

		// Créer les labels pour ce chemin
		diskLabels := cloneLabels(baseLabels)
		diskLabels["path"] = path

		// Ajouter les métriques
		batch.Metrics = append(batch.Metrics, []types.Metric{
			{
				Name:        metrics.SystemDiskUsage.Name,
				Value:       float64(usage.Used),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: diskLabels,
			},
			{
				Name:        metrics.SystemDiskFree.Name,
				Value:       float64(usage.Free),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: diskLabels,
			},
		}...)
	}

	// Envoyer les métriques
	if len(batch.Metrics) > 0 {
		if err := sm.vmClient.SendMetrics(batch); err != nil {
			utils.LogError("Failed to send disk metrics: %v", err)
		}
	}
}

// collectGoMetrics collecte les métriques Go
func (sm *SystemMonitor) collectGoMetrics() {
	// Obtenir les statistiques mémoire Go
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Créer les labels de base
	baseLabels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.SystemGoMemoryAlloc.Name,
				Value:       float64(memStats.Alloc),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemGoMemorySys.Name,
				Value:       float64(memStats.Sys),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemGoRoutines.Name,
				Value:       float64(runtime.NumGoroutine()),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Failed to send Go metrics: %v", err)
	}
}

// collectServerState collecte les métriques d'état du serveur
func (sm *SystemMonitor) collectServerState() {
	// Obtenir l'état du serveur
	sm.state.RLock()
	defer sm.state.RUnlock()

	// Créer les labels de base
	baseLabels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Déterminer l'état du serveur
	serverState := metrics.ServerStateStarting
	if sm.state.Ready {
		serverState = metrics.ServerStateReady
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerStateGauge.Name,
				Value:       float64(serverState),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.PlayersGauge.Name,
				Value:       float64(len(sm.state.ConnectedPlayers)),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.ServerUptime.Name,
				Value:       time.Since(sm.state.StartTime).Seconds(),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Failed to send server state metrics: %v", err)
	}
}

// CalculateNetworkLatency calcule la latence réseau vers un hôte
func (sm *SystemMonitor) CalculateNetworkLatency(host string) float64 {
	// Effectuer un ping simple en mesurant le temps de réponse
	start := time.Now()
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		utils.LogError("Failed to connect to %s: %v", host, err)
		return -1
	}
	defer conn.Close()
	return float64(time.Since(start).Milliseconds())
}

// MonitorNetworkLatency surveille la latence réseau vers un hôte
func (sm *SystemMonitor) MonitorNetworkLatency(ctx context.Context, host string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Calculer la latence
			latency := sm.CalculateNetworkLatency(host)
			if latency < 0 {
				continue
			}

			// Créer les labels
			labels := map[string]string{
				"server_id":   sm.state.ServerID,
				"server_name": sm.state.ServerName,
				"server_type": sm.state.ServerType,
				"target":      host,
			}

			// Envoyer la métrique
			batch := types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:        metrics.NetworkLatency.Name,
						Value:       latency,
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: labels,
					},
				},
				Time: time.Now(),
			}

			if err := sm.vmClient.SendMetrics(batch); err != nil {
				utils.LogError("Failed to send latency metrics: %v", err)
			}
		}
	}
}

// cloneLabels crée une copie d'une map de labels
func cloneLabels(labels map[string]string) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

// InitializeSystemMetrics initializes all system metrics with zero values
// to ensure they appear in the metrics system even before actual data is collected
func InitializeSystemMetrics(state *types.ServerState, vmClient *victoria.MetricsClient) {
	// Create base labels
	baseLabels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Create a batch of initial metrics
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			// CPU metrics
			{
				Name:        metrics.ServerCPUUsage.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemCPUUsage.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: addLabel(baseLabels, "cpu", "cpu0"),
			},
			// Memory metrics
			{
				Name:        metrics.ServerMemoryUsage.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemoryTotal.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemoryUsed.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemoryFree.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemMemorySwap.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Disk metrics
			{
				Name:        metrics.SystemDiskUsage.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: addLabel(baseLabels, "path", "/"),
			},
			{
				Name:        metrics.SystemDiskFree.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: addLabel(baseLabels, "path", "/"),
			},
			// Go runtime metrics
			{
				Name:        metrics.SystemGoMemoryAlloc.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemGoMemorySys.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.SystemGoRoutines.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Network metrics
			{
				Name:        metrics.NetworkLatency.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.NetworkPacketLoss.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Send the initial metrics
	if err := vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Failed to send initial system metrics: %v", err)
	} else {
		utils.LogInfo("Initial system metrics sent successfully")
	}
}

// addLabel creates a copy of the base labels and adds one additional label
func addLabel(baseLabels map[string]string, key, value string) map[string]string {
	result := make(map[string]string, len(baseLabels)+1)

	// Copy base labels
	for k, v := range baseLabels {
		result[k] = v
	}

	// Add additional label
	result[key] = value

	return result
}

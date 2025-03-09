package monitoring

import (
	"context"
	"fmt"
	stdnet "net"
	"os"
	"runtime"
	"time"

	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemMonitor surveille les métriques système (réseau, CPU, RAM)
type SystemMonitor struct {
	state       *types.ServerState
	vmClient    *victoria.MetricsClient
	process     *process.Process
	networkIfs  []string
	lastNetIO   map[string]net.IOCountersStat
	lastCPUTime time.Time
	diskPaths   []string
}

// NewSystemMonitor crée un nouveau moniteur système
func NewSystemMonitor(state *types.ServerState, vmClient *victoria.MetricsClient) (*SystemMonitor, error) {
	// Obtenir le PID du processus actuel
	pid := os.Getpid()
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création du moniteur système: %w", err)
	}

	// Obtenir les interfaces réseau
	ifaces, err := stdnet.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des interfaces réseau: %w", err)
	}

	// Filtrer les interfaces actives
	var activeIfs []string
	for _, iface := range ifaces {
		if iface.Flags&stdnet.FlagUp != 0 && iface.Flags&stdnet.FlagLoopback == 0 {
			activeIfs = append(activeIfs, iface.Name)
		}
	}

	// Initialiser les compteurs réseau
	ioCounters, err := net.IOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des compteurs réseau: %w", err)
	}

	lastNetIO := make(map[string]net.IOCountersStat)
	for _, io := range ioCounters {
		for _, name := range activeIfs {
			if io.Name == name {
				lastNetIO[name] = io
				break
			}
		}
	}

	// Déterminer les chemins de disque à surveiller
	diskPaths := []string{"/", "/var", "/tmp"}
	// Ajouter le répertoire de travail actuel
	wd, err := os.Getwd()
	if err == nil {
		diskPaths = append(diskPaths, wd)
	}

	return &SystemMonitor{
		state:       state,
		vmClient:    vmClient,
		process:     proc,
		networkIfs:  activeIfs,
		lastNetIO:   lastNetIO,
		lastCPUTime: time.Now(),
		diskPaths:   diskPaths,
	}, nil
}

// Start démarre la surveillance des métriques système
func (sm *SystemMonitor) Start(ctx context.Context) {
	// Collecter les métriques toutes les secondes
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Collecter les métriques de disque moins fréquemment
	diskTicker := time.NewTicker(30 * time.Second)
	defer diskTicker.Stop()

	// Collecter les métriques Go moins fréquemment
	goTicker := time.NewTicker(10 * time.Second)
	defer goTicker.Stop()

	// Collecter l'état du serveur régulièrement
	stateTicker := time.NewTicker(5 * time.Second)
	defer stateTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.collectNetworkMetrics()
			sm.collectCPUMetrics()
			sm.collectMemoryMetrics()
		case <-diskTicker.C:
			sm.collectDiskMetrics()
		case <-goTicker.C:
			sm.collectGoMetrics()
		case <-stateTicker.C:
			sm.collectServerState()
		}
	}
}

// collectNetworkMetrics collecte les métriques réseau
func (sm *SystemMonitor) collectNetworkMetrics() {
	// Obtenir les compteurs réseau actuels
	ioCounters, err := net.IOCounters(true)
	if err != nil {
		utils.LogError("Erreur lors de la récupération des compteurs réseau: %v", err)
		return
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{},
		Time:    time.Now(),
	}

	// Calculer les métriques pour chaque interface
	for _, io := range ioCounters {
		for _, name := range sm.networkIfs {
			if io.Name == name {
				// Vérifier si nous avons des données précédentes pour cette interface
				if lastIO, ok := sm.lastNetIO[name]; ok {
					// Calculer les octets reçus et envoyés depuis la dernière mesure
					bytesRecv := io.BytesRecv - lastIO.BytesRecv
					bytesSent := io.BytesSent - lastIO.BytesSent
					packetsRecv := io.PacketsRecv - lastIO.PacketsRecv
					packetsSent := io.PacketsSent - lastIO.PacketsSent
					errIn := io.Errin - lastIO.Errin
					errOut := io.Errout - lastIO.Errout
					dropIn := io.Dropin - lastIO.Dropin
					dropOut := io.Dropout - lastIO.Dropout

					// Créer les labels communs
					labels := map[string]string{
						"server_id":   sm.state.ServerID,
						"server_name": sm.state.ServerName,
						"server_type": sm.state.ServerType,
						"interface":   name,
					}

					// Ajouter les métriques au lot
					batch.Metrics = append(batch.Metrics,
						types.Metric{
							Name:        metrics.NetworkBytesReceived,
							Value:       float64(bytesRecv),
							Type:        types.Counter,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
						types.Metric{
							Name:        metrics.NetworkBytesSent,
							Value:       float64(bytesSent),
							Type:        types.Counter,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
						types.Metric{
							Name:        metrics.NetworkPacketsReceived,
							Value:       float64(packetsRecv),
							Type:        types.Counter,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
						types.Metric{
							Name:        metrics.NetworkPacketsSent,
							Value:       float64(packetsSent),
							Type:        types.Counter,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
						types.Metric{
							Name:        metrics.NetworkErrors,
							Value:       float64(errIn + errOut),
							Type:        types.Counter,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
						types.Metric{
							Name:        metrics.NetworkDrops,
							Value:       float64(dropIn + dropOut),
							Type:        types.Counter,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
					)

					// Calculer le taux de perte de paquets
					totalPackets := float64(packetsRecv + packetsSent)
					if totalPackets > 0 {
						packetLoss := float64(errIn+errOut+dropIn+dropOut) / totalPackets
						batch.Metrics = append(batch.Metrics, types.Metric{
							Name:        metrics.NetworkPacketLoss,
							Value:       packetLoss,
							Type:        types.Gauge,
							Timestamp:   time.Now(),
							LabelValues: labels,
						})
					}
				}

				// Mettre à jour les dernières valeurs
				sm.lastNetIO[name] = io
				break
			}
		}
	}

	// Envoyer les métriques
	if len(batch.Metrics) > 0 {
		if err := sm.vmClient.SendMetrics(batch); err != nil {
			utils.LogError("Erreur lors de l'envoi des métriques réseau: %v", err)
		}
	}
}

// collectCPUMetrics collecte les métriques CPU
func (sm *SystemMonitor) collectCPUMetrics() {
	// Obtenir l'utilisation CPU du processus
	cpuPercent, err := sm.process.CPUPercent()
	if err != nil {
		utils.LogError("Erreur lors de la récupération de l'utilisation CPU: %v", err)
		return
	}

	// Obtenir l'utilisation CPU globale
	systemCPU, err := cpu.Percent(0, false)
	if err != nil {
		utils.LogError("Erreur lors de la récupération de l'utilisation CPU système: %v", err)
		return
	}

	// Créer les labels
	labels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerCPUUsage,
				Value:       cpuPercent,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
			{
				Name:        metrics.SystemCPUUsage,
				Value:       systemCPU[0],
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	}

	// Obtenir l'utilisation CPU par cœur
	perCPU, err := cpu.Percent(0, true)
	if err == nil {
		for i, usage := range perCPU {
			coreLabels := cloneLabels(labels)
			coreLabels["core"] = fmt.Sprintf("%d", i)

			batch.Metrics = append(batch.Metrics, types.Metric{
				Name:        metrics.SystemCPUUsage,
				Value:       usage,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: coreLabels,
			})
		}
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Erreur lors de l'envoi des métriques CPU: %v", err)
	}
}

// collectMemoryMetrics collecte les métriques mémoire
func (sm *SystemMonitor) collectMemoryMetrics() {
	// Obtenir l'utilisation mémoire du processus
	memInfo, err := sm.process.MemoryInfo()
	if err != nil {
		utils.LogError("Erreur lors de la récupération de l'utilisation mémoire: %v", err)
		return
	}

	// Obtenir l'utilisation mémoire globale
	systemMem, err := mem.VirtualMemory()
	if err != nil {
		utils.LogError("Erreur lors de la récupération de l'utilisation mémoire système: %v", err)
		return
	}

	// Obtenir l'utilisation swap
	swapMem, err := mem.SwapMemory()
	if err != nil {
		utils.LogWarning("Erreur lors de la récupération de l'utilisation swap: %v", err)
	}

	// Créer les labels
	labels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerMemoryUsage,
				Value:       float64(memInfo.RSS),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
			{
				Name:        metrics.SystemMemoryTotal,
				Value:       float64(systemMem.Total),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
			{
				Name:        metrics.SystemMemoryUsed,
				Value:       float64(systemMem.Used),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
			{
				Name:        metrics.SystemMemoryFree,
				Value:       float64(systemMem.Free),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	}

	// Ajouter les métriques swap si disponibles
	if swapMem != nil {
		batch.Metrics = append(batch.Metrics, types.Metric{
			Name:        metrics.SystemMemorySwap,
			Value:       float64(swapMem.Used),
			Type:        types.Gauge,
			Timestamp:   time.Now(),
			LabelValues: labels,
		})
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Erreur lors de l'envoi des métriques mémoire: %v", err)
	}
}

// collectDiskMetrics collecte les métriques d'utilisation du disque
func (sm *SystemMonitor) collectDiskMetrics() {
	// Créer les labels communs
	labels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{},
		Time:    time.Now(),
	}

	// Collecter les métriques pour chaque chemin
	for _, path := range sm.diskPaths {
		usage, err := disk.Usage(path)
		if err != nil {
			utils.LogWarning("Erreur lors de la récupération de l'utilisation disque pour %s: %v", path, err)
			continue
		}

		// Créer des labels spécifiques pour ce chemin
		pathLabels := cloneLabels(labels)
		pathLabels["path"] = path

		// Ajouter les métriques
		batch.Metrics = append(batch.Metrics,
			types.Metric{
				Name:        metrics.SystemDiskUsage,
				Value:       float64(usage.Used),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: pathLabels,
			},
			types.Metric{
				Name:        metrics.SystemDiskFree,
				Value:       float64(usage.Free),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: pathLabels,
			},
		)
	}

	// Envoyer les métriques
	if len(batch.Metrics) > 0 {
		if err := sm.vmClient.SendMetrics(batch); err != nil {
			utils.LogError("Erreur lors de l'envoi des métriques disque: %v", err)
		}
	}
}

// collectGoMetrics collecte les métriques de l'environnement Go
func (sm *SystemMonitor) collectGoMetrics() {
	// Obtenir les statistiques de la mémoire Go
	var goMemStats runtime.MemStats
	runtime.ReadMemStats(&goMemStats)

	// Créer les labels
	labels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.SystemGoMemoryAlloc,
				Value:       float64(goMemStats.Alloc),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
			{
				Name:        metrics.SystemGoMemorySys,
				Value:       float64(goMemStats.Sys),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
			{
				Name:        metrics.SystemGoRoutines,
				Value:       float64(runtime.NumGoroutine()),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Erreur lors de l'envoi des métriques Go: %v", err)
	}
}

// collectServerState collecte et envoie la métrique d'état du serveur
func (sm *SystemMonitor) collectServerState() {
	// Créer les labels
	labels := map[string]string{
		"server_id":   sm.state.ServerID,
		"server_name": sm.state.ServerName,
		"server_type": sm.state.ServerType,
	}

	// Déterminer l'état actuel du serveur
	var serverState int
	sm.state.RLock()
	if sm.state.ShuttingDown {
		serverState = types.ServerStateShutdown
	} else if sm.state.Ready {
		serverState = types.ServerStateReady
	} else if sm.state.Allocated {
		serverState = types.ServerStateAllocated
	} else {
		serverState = types.ServerStateStarting
	}
	sm.state.RUnlock()

	// Créer un lot de métriques
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerStateGauge.Name,
				Value:       float64(serverState),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	}

	// Envoyer les métriques
	if err := sm.vmClient.SendMetrics(batch); err != nil {
		utils.LogError("Erreur lors de l'envoi de la métrique d'état du serveur: %v", err)
	} else {
		utils.LogDebug("Métrique d'état du serveur envoyée: %d", serverState)
	}
}

// CalculateNetworkLatency calcule la latence réseau en effectuant un ping
func (sm *SystemMonitor) CalculateNetworkLatency(host string) float64 {
	// Implémenter un ping simple pour mesurer la latence
	start := time.Now()
	conn, err := stdnet.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		utils.LogWarning("Erreur lors du ping vers %s: %v", host, err)
		return 0
	}
	defer conn.Close()

	latency := time.Since(start).Milliseconds()
	return float64(latency)
}

// MonitorNetworkLatency surveille la latence réseau vers un hôte spécifique
func (sm *SystemMonitor) MonitorNetworkLatency(ctx context.Context, host string) {
	// Collecter la latence toutes les 5 secondes
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			latency := sm.CalculateNetworkLatency(host)
			if latency > 0 {
				// Créer les labels
				labels := map[string]string{
					"server_id":   sm.state.ServerID,
					"server_name": sm.state.ServerName,
					"server_type": sm.state.ServerType,
					"target_host": host,
				}

				// Envoyer la métrique
				batch := types.MetricBatch{
					Metrics: []types.Metric{
						{
							Name:        metrics.NetworkLatency,
							Value:       latency,
							Type:        types.Gauge,
							Timestamp:   time.Now(),
							LabelValues: labels,
						},
					},
					Time: time.Now(),
				}

				if err := sm.vmClient.SendMetrics(batch); err != nil {
					utils.LogError("Erreur lors de l'envoi de la métrique de latence: %v", err)
				}
			}
		}
	}
}

// copyLabels crée une copie des labels
func cloneLabels(labels map[string]string) map[string]string {
	newLabels := make(map[string]string)
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

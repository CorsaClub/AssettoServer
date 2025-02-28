package monitoring

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"metrics/models"
	metrics "metrics/services"
	"metrics/utils"
	"metrics/victoria"
)

// LogMonitor surveille les fichiers de log du serveur
type LogMonitor struct {
	vmClient    *victoria.Client
	logDir      string
	logPatterns []string
	filters     map[string]*regexp.Regexp
	seenLogs    map[string]time.Time // Clé: message du log, Valeur: timestamp
	batchSize   int
	batchTimer  time.Duration
	breaker     *utils.CircuitBreaker
}

// Add these option functions
type LogMonitorOption func(*LogMonitor)

func WithPatterns(patterns []string) LogMonitorOption {
	return func(lm *LogMonitor) {
		lm.logPatterns = patterns
	}
}

func WithMaxFileSize(size int64) LogMonitorOption {
	return func(lm *LogMonitor) {
		// Implementation for max file size
	}
}

func NewLogMonitor(vmClient *victoria.Client, logDir string, opts ...LogMonitorOption) *LogMonitor {
	lm := &LogMonitor{
		vmClient: vmClient,
		logDir:   logDir,
		logPatterns: []string{
			"error.log",
			"server.log",
			"access.log",
			"chat.log",
			"admin.log",
			"connection.log",
		},
		filters:    initLogFilters(),
		seenLogs:   make(map[string]time.Time),
		batchSize:  100,
		batchTimer: 5 * time.Second,
		breaker:    utils.NewCircuitBreaker(3, 30*time.Second), // 3 max failures, 30s timeout
	}

	for _, opt := range opts {
		opt(lm)
	}

	return lm
}

func initLogFilters() map[string]*regexp.Regexp {
	return map[string]*regexp.Regexp{
		"error":    regexp.MustCompile(`(?i)(error|exception|failed|crash)`),
		"warning":  regexp.MustCompile(`(?i)(warning|warn|attention)`),
		"critical": regexp.MustCompile(`(?i)(critical|fatal|emergency|panic)`),
		"session":  regexp.MustCompile(`(?i)(session.*started|session.*ended|practice|qualifying|race)`),
		"player":   regexp.MustCompile(`(?i)(connected|disconnected|kicked|banned|collision)`),
		"network":  regexp.MustCompile(`(?i)(timeout|connection.*refused|network.*error|latency)`),
		"system":   regexp.MustCompile(`(?i)(cpu|memory|disk|bandwidth|resource)`),
	}
}

func (lm *LogMonitor) Start(ctx context.Context) {
	// Créer le répertoire de logs s'il n'existe pas
	if err := os.MkdirAll(lm.logDir, 0755); err != nil {
		utils.LogError("Failed to create log directory: %v", err)
		return
	}

	// Démarrer la surveillance pour chaque fichier de log
	for _, pattern := range lm.logPatterns {
		go lm.monitorLog(ctx, pattern)
	}
}

func (lm *LogMonitor) monitorLog(ctx context.Context, pattern string) {
	logPath := filepath.Join(lm.logDir, pattern)
	file, err := os.Open(logPath)
	if err != nil {
		utils.LogError("Failed to open log file %s: %v", logPath, err)
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	logs := make([]models.LogEntry, 0, lm.batchSize)
	ticker := time.NewTicker(lm.batchTimer)
	for {
		select {
		case <-ctx.Done():
			lm.processBatch(logs)
			return
		case <-ticker.C:
			lm.processBatch(logs)
			logs = make([]models.LogEntry, 0, lm.batchSize)
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				time.Sleep(time.Second)
				continue
			}

			// Analyser le log
			logLevel, eventType := lm.analyzeLine(line)
			if logLevel != "" {
				logs = append(logs, models.LogEntry{
					Level:   logLevel,
					Message: line,
					Labels: map[string]string{
						"log_file":   pattern,
						"source":     "assetto_server",
						"event_type": eventType,
					},
				})
			}

			if len(logs) >= lm.batchSize {
				lm.processBatch(logs)
				logs = make([]models.LogEntry, 0, lm.batchSize)
			}
		}
	}
}

func (lm *LogMonitor) analyzeLine(line string) (logLevel, eventType string) {
	// Vérifier les patterns critiques en premier
	if lm.filters["critical"].MatchString(line) {
		return metrics.LogLevelError, "critical_event"
	}

	// Vérifier les erreurs
	if lm.filters["error"].MatchString(line) {
		return metrics.LogLevelError, "error_event"
	}

	// Vérifier les avertissements
	if lm.filters["warning"].MatchString(line) {
		return metrics.LogLevelWarning, "warning_event"
	}

	// Vérifier les événements de session
	if lm.filters["session"].MatchString(line) {
		return metrics.LogLevelInfo, "session_event"
	}

	// Vérifier les événements joueurs
	if lm.filters["player"].MatchString(line) {
		return metrics.LogLevelInfo, "player_event"
	}

	// Vérifier les événements réseau
	if lm.filters["network"].MatchString(line) {
		return metrics.LogLevelWarning, "network_event"
	}

	// Vérifier les événements système
	if lm.filters["system"].MatchString(line) {
		return metrics.LogLevelInfo, "system_event"
	}

	return "", ""
}

func (lm *LogMonitor) processBatch(logs []models.LogEntry) {
	if len(logs) == 0 {
		return
	}

	now := time.Now()
	// Nettoyer les entrées plus vieilles que 1 heure
	for msg, timestamp := range lm.seenLogs {
		if now.Sub(timestamp) > time.Hour {
			delete(lm.seenLogs, msg)
		}
	}

	// Dédupliquer les logs similaires
	deduped := make(map[string]models.LogEntry)
	for _, log := range logs {
		// Utiliser le message comme clé de déduplication
		if _, seen := lm.seenLogs[log.Message]; !seen {
			deduped[log.Message] = log
			lm.seenLogs[log.Message] = now
		}
	}

	// Envoyer les logs dédupliqués
	if len(deduped) > 0 {
		values := make([]models.LogEntry, 0, len(deduped))
		for _, v := range deduped {
			values = append(values, v)
		}
		lm.vmClient.SendLogs(values)
	}
}

func (lm *LogMonitor) recoverLostLogs() error {
	// Implémentation de la récupération
	return nil
}

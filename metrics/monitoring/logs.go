package monitoring

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"metrics/models"
	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

// LogMonitorOption est une fonction qui configure un LogMonitor
type LogMonitorOption func(*LogMonitor)

// WithPatterns définit les patterns de fichiers à surveiller
func WithPatterns(patterns []string) LogMonitorOption {
	return func(lm *LogMonitor) {
		lm.logPatterns = patterns
	}
}

// WithMaxFileSize définit la taille maximale des fichiers à surveiller
func WithMaxFileSize(maxSize int64) LogMonitorOption {
	return func(lm *LogMonitor) {
		lm.maxFileSize = maxSize
	}
}

// WithLineCallback définit une fonction de rappel pour chaque ligne lue
func WithLineCallback(callback func(string)) LogMonitorOption {
	return func(lm *LogMonitor) {
		lm.lineCallback = callback
	}
}

// LogMonitor surveille les fichiers de log du serveur
type LogMonitor struct {
	metricsClient  *victoria.MetricsClient
	logsClient     victoria.LogsClient
	logDir         string
	logPatterns    []string
	filters        map[string]*regexp.Regexp
	seenLogs       map[string]time.Time // Clé: message du log, Valeur: timestamp
	batchSize      int
	batchTimer     time.Duration
	breaker        *utils.CircuitBreaker
	chatEnabled    bool
	incidentTypes  map[string]*regexp.Regexp
	configPatterns map[string]*regexp.Regexp
	maxFileSize    int64
	lineCallback   func(string)
}

// NewLogMonitor crée un nouveau moniteur de logs
func NewLogMonitor(
	metricsClient *victoria.MetricsClient,
	logsClient victoria.LogsClient,
	directory string,
	opts ...LogMonitorOption,
) *LogMonitor {
	lm := &LogMonitor{
		metricsClient: metricsClient,
		logsClient:    logsClient,
		logDir:        directory,
		logPatterns: []string{
			"error.log",
			"server.log",
			"access.log",
			"chat.log",
			"admin.log",
			"connection.log",
		},
		filters:        initLogFilters(),
		seenLogs:       make(map[string]time.Time),
		batchSize:      100,
		batchTimer:     5 * time.Second,
		breaker:        utils.NewCircuitBreaker(3, 30*time.Second), // 3 max failures, 30s timeout
		chatEnabled:    false,
		incidentTypes:  make(map[string]*regexp.Regexp),
		configPatterns: make(map[string]*regexp.Regexp),
		maxFileSize:    100 * 1024 * 1024, // 100MB par défaut
	}

	for _, opt := range opts {
		opt(lm)
	}

	return lm
}

func initLogFilters() map[string]*regexp.Regexp {
	filters := map[string]*regexp.Regexp{
		"error":      regexp.MustCompile(`(?i)(error|exception|failed|crash)`),
		"warning":    regexp.MustCompile(`(?i)(warning|warn|attention)`),
		"critical":   regexp.MustCompile(`(?i)(critical|fatal|emergency|panic)`),
		"session":    regexp.MustCompile(`(?i)(session.*started|session.*ended|practice|qualifying|race|Starting session|Session ended|Session type changed)`),
		"player":     regexp.MustCompile(`(?i)(connected|disconnected|kicked|banned|collision)`),
		"network":    regexp.MustCompile(`(?i)(timeout|connection.*refused|network.*error|latency)`),
		"system":     regexp.MustCompile(`(?i)(cpu|memory|disk|bandwidth|resource)`),
		"connection": regexp.MustCompile(`(?i)(attempting to connect|supports extra CSP features)`),
		"chat":       regexp.MustCompile(`CHAT:`),
		"exit":       regexp.MustCompile(`(?i)(Clean exit received|disconnected)`),
		"csp":        regexp.MustCompile(`(?i)(CSP handshake|CSP features enabled)`),
	}

	return filters
}

// Start démarre la surveillance des logs
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

	// Ajouter l'analyse des nouveaux types de logs
	if lm.chatEnabled && strings.Contains(line, "CHAT:") {
		return metrics.LogLevelInfo, "chat_message"
	}

	// Analyse des connexions
	if lm.filters["connection"].MatchString(line) {
		if strings.Contains(line, "attempting to connect") {
			return metrics.LogLevelInfo, "connection_attempt"
		}
		if strings.Contains(line, "supports extra CSP features") {
			return metrics.LogLevelInfo, "csp_features"
		}
	}

	// Analyse des sessions
	if lm.filters["session"].MatchString(line) {
		return metrics.LogLevelInfo, "session_change"
	}

	// Analyse des déconnexions propres
	if lm.filters["exit"].MatchString(line) {
		return metrics.LogLevelInfo, "clean_exit"
	}

	// Détecter les incidents
	for incidentType, pattern := range lm.incidentTypes {
		if pattern.MatchString(line) {
			return metrics.LogLevelWarning, "race_incident_" + incidentType
		}
	}

	// Détecter les changements de configuration
	for configType, pattern := range lm.configPatterns {
		if pattern.MatchString(line) {
			return metrics.LogLevelInfo, "config_change_" + configType
		}
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
		// Convert models.LogEntry to types.Log
		logs := make([]types.Log, len(values))
		for i, v := range values {
			logs[i] = types.Log{
				Timestamp: v.Timestamp,
				Level:     v.Level,
				Message:   v.Message,
				Labels:    v.Labels,
				Source:    "acserver",
			}
		}
		lm.logsClient.SendLogs(logs)
	}
}

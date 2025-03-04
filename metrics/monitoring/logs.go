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

	// Vérifier si le fichier existe, sinon attendre qu'il soit créé
	for {
		if _, err := os.Stat(logPath); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			utils.LogInfo("Waiting for log file to be created: %s", logPath)
		}
	}

	utils.LogInfo("Monitoring log file: %s", logPath)

	// Ouvrir le fichier et se positionner à la fin
	file, err := os.Open(logPath)
	if err != nil {
		utils.LogError("Failed to open log file %s: %v", logPath, err)
		return
	}
	defer file.Close()

	// Se positionner à la fin du fichier pour ne lire que les nouvelles entrées
	if _, err := file.Seek(0, 2); err != nil {
		utils.LogError("Failed to seek to end of file %s: %v", logPath, err)
		return
	}

	reader := bufio.NewReader(file)
	logs := make([]models.LogEntry, 0, lm.batchSize)
	ticker := time.NewTicker(lm.batchTimer)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if len(logs) > 0 {
				lm.processBatch(logs)
			}
			return
		case <-ticker.C:
			if len(logs) > 0 {
				lm.processBatch(logs)
				logs = make([]models.LogEntry, 0, lm.batchSize)
			}
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				// Si EOF, attendre de nouvelles données
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Nettoyer la ligne
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Appeler le callback si défini
			if lm.lineCallback != nil {
				lm.lineCallback(line)
			}

			// Analyser le log
			logLevel, eventType := lm.analyzeLine(line)
			if logLevel != "" {
				timestamp := time.Now()
				logs = append(logs, models.LogEntry{
					Timestamp: timestamp,
					Level:     logLevel,
					Message:   line,
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

	// Convertir les logs en format VictoriaLogs
	victoriaLogs := make([]types.Log, 0, len(logs))

	for _, log := range logs {
		// Ajouter uniquement les logs qui n'ont pas été vus récemment
		if _, seen := lm.seenLogs[log.Message]; !seen {
			lm.seenLogs[log.Message] = time.Now()

			// Créer un log au format attendu par VictoriaLogs
			victoriaLog := types.Log{
				Timestamp: log.Timestamp,
				Level:     log.Level,
				Message:   log.Message,
				Source:    log.Labels["log_file"],
				Labels:    make(map[string]string),
			}

			// Copier tous les labels
			for k, v := range log.Labels {
				victoriaLog.Labels[k] = v
			}

			victoriaLogs = append(victoriaLogs, victoriaLog)
		}
	}

	// Envoyer les logs à VictoriaLogs
	if len(victoriaLogs) > 0 {
		if err := lm.logsClient.SendLogs(victoriaLogs); err != nil {
			utils.LogError("Failed to send logs to VictoriaLogs: %v", err)
		} else {
			utils.LogInfo("Successfully sent %d logs to VictoriaLogs", len(victoriaLogs))
		}
	}
}

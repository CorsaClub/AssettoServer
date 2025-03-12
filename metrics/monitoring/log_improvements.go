package monitoring

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"metrics/types"
	"metrics/utils"
)

// LoggingMetrics contient toutes les métriques liées au logging
type LoggingMetrics struct {
	batchSize      atomic.Int64
	processingTime *utils.TimeHistogram
	failedAttempts atomic.Int64
	bytesProcessed atomic.Int64
}

// NewLoggingMetrics crée et initialise les métriques de logging
func NewLoggingMetrics() *LoggingMetrics {
	return &LoggingMetrics{
		processingTime: utils.NewTimeHistogram([]float64{
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
		}),
	}
}

// GetMetrics retourne les métriques actuelles
func (m *LoggingMetrics) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"batch_size":      m.batchSize.Load(),
		"processing_time": m.processingTime.GetMetrics(),
		"failed_attempts": m.failedAttempts.Load(),
		"bytes_processed": m.bytesProcessed.Load(),
	}
}

// Pool d'objets pour les entrées de log
var logEntryPool = sync.Pool{
	New: func() interface{} {
		return &types.LogEntry{
			Labels: make(map[string]string),
		}
	},
}

// AcquireLogEntry obtient une entrée de log du pool
func AcquireLogEntry() *types.LogEntry {
	return logEntryPool.Get().(*types.LogEntry)
}

// ReleaseLogEntry retourne une entrée de log au pool
func ReleaseLogEntry(entry *types.LogEntry) {
	// Nettoyer l'entrée
	entry.Message = ""
	entry.Level = ""
	entry.ServerID = ""
	entry.SessionID = ""
	entry.PlayerID = ""
	entry.PlayerName = ""
	entry.EventType = ""
	entry.Error = ""
	for k := range entry.Labels {
		delete(entry.Labels, k)
	}
	logEntryPool.Put(entry)
}

// ValidateLogEntry valide une entrée de log
func ValidateLogEntry(entry *types.LogEntry) error {
	if entry == nil {
		return errors.New("log entry is nil")
	}
	if entry.Message == "" {
		return errors.New("empty message")
	}
	if entry.Level == "" {
		return errors.New("missing log level")
	}
	if entry.Timestamp.IsZero() {
		return errors.New("missing timestamp")
	}
	return nil
}

// RotateLogFile effectue la rotation d'un fichier de log
func RotateLogFile(logPath string, maxSize int64) error {
	// Vérifier si le fichier existe
	info, err := os.Stat(logPath)
	if err != nil {
		return err
	}

	// Vérifier si la rotation est nécessaire
	if info.Size() < maxSize {
		return nil
	}

	// Créer le nom du fichier de backup
	timestamp := time.Now().Format("20060102-150405")
	backupPath := logPath + "." + timestamp

	// Renommer le fichier actuel
	if err := os.Rename(logPath, backupPath); err != nil {
		return err
	}

	// Créer un nouveau fichier vide
	file, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Compresser le fichier de backup en arrière-plan
	go func() {
		if err := utils.CompressFile(backupPath); err != nil {
			utils.LogError("Failed to compress rotated log file: %v", err)
		}
	}()

	return nil
}

// PurgeOldLogs supprime les vieux fichiers de log
func PurgeOldLogs(logDir string, maxAge time.Duration) error {
	now := time.Now()

	return filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignorer les répertoires
		if info.IsDir() {
			return nil
		}

		// Vérifier l'âge du fichier
		if now.Sub(info.ModTime()) > maxAge {
			if err := os.Remove(path); err != nil {
				utils.LogError("Failed to remove old log file %s: %v", path, err)
				return err
			}
			utils.LogInfo("Removed old log file: %s", path)
		}

		return nil
	})
}

// LogRotator gère la rotation automatique des logs
type LogRotator struct {
	maxSize  int64
	maxAge   time.Duration
	logDir   string
	interval time.Duration
	stopChan chan struct{}
	metrics  *LoggingMetrics
}

// NewLogRotator crée un nouveau gestionnaire de rotation des logs
func NewLogRotator(logDir string, maxSize int64, maxAge time.Duration, metrics *LoggingMetrics) *LogRotator {
	return &LogRotator{
		maxSize:  maxSize,
		maxAge:   maxAge,
		logDir:   logDir,
		interval: time.Minute * 5, // Vérifier toutes les 5 minutes
		stopChan: make(chan struct{}),
		metrics:  metrics,
	}
}

// Start démarre le processus de rotation automatique
func (r *LogRotator) Start() {
	ticker := time.NewTicker(r.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				r.rotateAll()
				r.purgeOld()
			case <-r.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop arrête le processus de rotation
func (r *LogRotator) Stop() {
	close(r.stopChan)
}

// rotateAll effectue la rotation de tous les fichiers de log si nécessaire
func (r *LogRotator) rotateAll() {
	files, err := filepath.Glob(filepath.Join(r.logDir, "*.log"))
	if err != nil {
		utils.LogError("Failed to list log files: %v", err)
		return
	}

	for _, file := range files {
		if err := RotateLogFile(file, r.maxSize); err != nil {
			utils.LogError("Failed to rotate log file %s: %v", file, err)
			r.metrics.failedAttempts.Add(1)
		}
	}
}

// purgeOld supprime les vieux fichiers de log
func (r *LogRotator) purgeOld() {
	if err := PurgeOldLogs(r.logDir, r.maxAge); err != nil {
		utils.LogError("Failed to purge old logs: %v", err)
		r.metrics.failedAttempts.Add(1)
	}
}

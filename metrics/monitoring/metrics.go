package monitoring

import (
	"context"
	"metrics/types"
	"metrics/victoria"
)

func MonitorMetrics(ctx context.Context, vmClient *victoria.Client, state *types.ServerState) {
	// Implementation of the function
}

func MonitorMetricsSystem(ctx context.Context, vmClient *victoria.Client) {
	// Monitorer la latence d'envoi
	// Monitorer le taux de succès
	// Monitorer la taille des batchs
	// Monitorer les erreurs
}

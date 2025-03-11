package handlers

import (
	"fmt"

	"metrics/metrics"
	"metrics/utils"
)

// handleNetworkStats updates network-related metrics based on the server output.
func handleNetworkStats(output string, labels map[string]string) {
	// Extract network statistics
	bytesReceived := utils.ExtractBytesReceived(output)
	bytesSent := utils.ExtractBytesSent(output)
	packetLoss := utils.ExtractPacketLoss(output)
	latency := utils.ExtractLatency(output)

	// Update bytes received counter
	if bytesReceived > 0 {
		metrics.NetworkBytesReceivedCounter.With(labels).Add(float64(bytesReceived))
	}

	// Update bytes sent counter
	if bytesSent > 0 {
		metrics.NetworkBytesSentCounter.With(labels).Add(float64(bytesSent))
	}

	// Update packet loss gauge
	if packetLoss > 0 {
		metrics.PacketLossGauge.With(labels).Set(packetLoss)
	}

	// Update latency gauge
	if latency > 0 {
		metrics.PlayerLatencyGauge.With(labels).Set(latency)
	}

	// Log network stats
	logEvent("network_stats", fmt.Sprintf("Network stats: Received=%d bytes, Sent=%d bytes, Loss=%.2f%%, Latency=%.2f ms",
		bytesReceived, bytesSent, packetLoss*100, latency), nil, map[string]string{
		"server_id":      labels["server_id"],
		"server_name":    labels["server_name"],
		"server_type":    labels["server_type"],
		"bytes_received": fmt.Sprintf("%d", bytesReceived),
		"bytes_sent":     fmt.Sprintf("%d", bytesSent),
		"packet_loss":    fmt.Sprintf("%.4f", packetLoss),
		"latency_ms":     fmt.Sprintf("%.2f", latency),
	})
}

package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	sdk "agones.dev/agones/sdks/go"
)

type interceptor struct {
	forward   io.Writer
	intercept func(p []byte)
}

func (i *interceptor) Write(p []byte) (n int, err error) {
	if i.intercept != nil {
		i.intercept(p)
	}
	return i.forward.Write(p)
}

func main() {
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")
	flag.Parse()

	argsList := strings.Fields(*args)
	log.Println(">>> Connecting to Agones with the SDK")
	s, err := sdk.NewSDK()
	if err != nil {
		log.Fatalf(">>> Could not connect to sdk: %v", err)
	}

	// Démarrer le health checking
	log.Println(">>> Starting health checking")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go doHealth(ctx, s)

	// Préparer la commande
	cmd := exec.CommandContext(ctx, *input, argsList...)
	cmd.Stderr = &interceptor{forward: os.Stderr}

	serverReady := make(chan struct{}, 1)
	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))
			if strings.Contains(str, "Starting Assetto Corsa Server...") {
				log.Println(">>> Server starting up...")
			} else if strings.Contains(str, "Lobby registration successful") {
				log.Println(">>> Server is ready")
				serverReady <- struct{}{}
			} else if strings.Contains(str, "timeleft") {
				log.Println(">>> End of session. Shutting down server.")
				if err := s.Shutdown(); err != nil {
					log.Printf(">>> Warning: Could not send shutdown message: %v", err)
				}
			}
		}}

	log.Printf(">>> Starting server script: %s %v\n", *input, argsList)

	// Démarrer le serveur
	if err := cmd.Start(); err != nil {
		log.Fatalf(">>> Error Starting Cmd: %v", err)
	}

	// Gérer les signaux de terminaison
	go handleSignals(cancel, s)

	// Attendre que le serveur soit prêt (avec un timeout de 5 minutes)
	select {
	case <-serverReady:
		log.Println(">>> Server reported ready, marking GameServer as Ready")
		if err := s.Ready(); err != nil {
			log.Printf(">>> Warning: Could not send ready message: %v", err)
		}
	case <-time.After(5 * time.Minute):
		log.Println(">>> Timeout waiting for server to be ready")
		if err := s.Shutdown(); err != nil {
			log.Printf(">>> Warning: Could not send shutdown message: %v", err)
		}
		cancel()
	}

	// Attendre la fin du serveur
	if err := cmd.Wait(); err != nil {
		if err := s.Shutdown(); err != nil {
			log.Printf(">>> Warning: Could not send shutdown message: %v", err)
		}
		log.Fatalf(">>> Server exited unexpectedly: %v", err)
	}
}

func doHealth(ctx context.Context, s *sdk.SDK) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Health(); err != nil {
				log.Printf(">>> Warning: Health ping failed: %v", err)
			}
		}
	}
}

func handleSignals(cancel context.CancelFunc, s *sdk.SDK) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println(">>> Received termination signal. Shutting down gracefully.")
	if err := s.Shutdown(); err != nil {
		log.Printf(">>> Warning: Could not send shutdown message: %v", err)
	}
	cancel()
}

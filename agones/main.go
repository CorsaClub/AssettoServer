package main

import (
	"context"
	"flag"
	"fmt"
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
	input := flag.String("i", "./acServer", "Path to Assetto Corsa server executable")
	args := flag.String("args", "--plugins-from-workdir", "Arguments for the server")
	flag.Parse()

	argsList := strings.Fields(*args)
	fmt.Println(">>> Connecting to Agones with the SDK")
	s, err := sdk.NewSDK()
	if err != nil {
		log.Fatalf(">>> Could not connect to sdk: %v", err)
	}

	fmt.Println(">>> Starting health checking")
	go doHealth(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, *input, argsList...)
	cmd.Stderr = &interceptor{forward: os.Stderr}
	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))
			if strings.Contains(str, "Lobby registration successful") {
				err := s.Ready()
				if err != nil {
					log.Fatalf(">>> Could not send ready message")
				}
				fmt.Println(">>> Assetto Corsa server is ready!")
			} else if strings.Contains(str, "timeleft") {
				fmt.Println(">>> End of session. Shutting down server.")
				err := s.Shutdown()
				if err != nil {
					log.Fatalf(">>> Could not send shutdown message")
				}
			}
		}}

	fmt.Printf(">>> Starting Assetto Corsa server: %s %v\n", *input, argsList)

	err = cmd.Start()
	if err != nil {
		log.Fatalf(">>> Error Starting Cmd: %v", err)
	}

	go handleSignals(cancel, s)

	err = cmd.Wait()
	log.Fatalf(">>> Assetto Corsa server exited unexpectedly: %v", err)
}

func doHealth(sdk *sdk.SDK) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := sdk.Health(); err != nil {
			log.Printf("[wrapper] Health ping failed: %v", err)
		}
	}
}

func handleSignals(cancel context.CancelFunc, s *sdk.SDK) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	fmt.Println(">>> Received termination signal. Shutting down gracefully.")
	s.Shutdown()
	cancel()
}

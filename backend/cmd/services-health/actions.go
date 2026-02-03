package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	startWaitTimeout  = 10 * time.Second
	startPollInterval = 200 * time.Millisecond
	maxStartOutput    = 400
)

type actionResult struct {
	notice       string
	serviceID    string
	instanceName string
	desiredUp    *bool
}

func (u *uiServer) runAction(serviceID, instanceName, configInput, addrInput string, isStart bool) actionResult {
	service, instance, err := findServiceInstance(u.config, serviceID, instanceName)
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	configPath := strings.TrimSpace(configInput)
	addr := strings.TrimSpace(addrInput)
	if configPath == "" {
		configPath = defaultConfigPathFor(service, instance)
	}
	if addr == "" {
		addr = instance.Addr
	}
	_, port, err := splitAddr(addr)
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	var cmdArgs []string
	if isStart {
		cmdArgs, err = buildCommand(service.StartCommand, configPath, addr, port)
	} else {
		cmdArgs, err = buildCommand(service.StopCommand, configPath, addr, port)
	}
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var output bytes.Buffer
	writer := io.MultiWriter(os.Stderr, &output)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if isStart {
		if err := cmd.Start(); err != nil {
			return actionResult{notice: formatStartError(err, output.String()), serviceID: serviceID, instanceName: instanceName}
		}
		exitCh := make(chan error, 1)
		// Wait in a goroutine so we can poll health while still observing early process exits.
		go func() {
			exitCh <- cmd.Wait()
		}()
		healthURL, err := resolveHealthURL(instance)
		if err != nil {
			return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
		}
		if err := waitForHealthUp(healthURL, startWaitTimeout, exitCh); err != nil {
			return actionResult{notice: formatStartError(err, output.String()), serviceID: serviceID, instanceName: instanceName}
		}
		log.Printf("started %s/%s: %s", service.ID, instance.Name, strings.Join(cmdArgs, " "))
		return actionResult{notice: fmt.Sprintf("started %s (%s)", service.Label, instance.Name), serviceID: service.ID, instanceName: instance.Name, desiredUp: boolPtr(true)}
	}
	if err := cmd.Run(); err != nil {
		return actionResult{notice: fmt.Sprintf("stop failed: %v", err), serviceID: serviceID, instanceName: instanceName}
	}
	healthURL, err := resolveHealthURL(instance)
	if err != nil {
		return actionResult{notice: err.Error(), serviceID: serviceID, instanceName: instanceName}
	}
	if err := waitForHealthDown(healthURL, startWaitTimeout); err != nil {
		return actionResult{notice: fmt.Sprintf("stop failed: %v", err), serviceID: serviceID, instanceName: instanceName}
	}
	log.Printf("stopped %s/%s: %s", service.ID, instance.Name, strings.Join(cmdArgs, " "))
	return actionResult{notice: fmt.Sprintf("stopped %s (%s)", service.Label, instance.Name), serviceID: service.ID, instanceName: instance.Name, desiredUp: boolPtr(false)}
}

func formatStartError(err error, output string) string {
	message := strings.TrimSpace(output)
	if message != "" {
		message = summarizeOutput(message)
		return fmt.Sprintf("start failed: %v: %s", err, message)
	}
	return fmt.Sprintf("start failed: %v", err)
}

func summarizeOutput(output string) string {
	output = strings.ReplaceAll(output, "\n", " ")
	output = strings.TrimSpace(output)
	if len(output) > maxStartOutput {
		return output[:maxStartOutput] + "..."
	}
	return output
}

func boolPtr(value bool) *bool {
	return &value
}

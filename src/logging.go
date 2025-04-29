package src

import (
	"log/syslog"
	"os/user"
	"fmt"
)

// logToSyslog sends a log message to syslog
func LogToSyslog(message string) error {
	logger, err := syslog.New(syslog.LOG_INFO, "kstool")
	if err != nil {
		return fmt.Errorf("failed to connect to syslog: %v", err)
	}
	defer logger.Close()

	return logger.Info(message)
}

// getCurrentUser returns the current user's username
func GetCurrentUser() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %v", err)
	}
	return currentUser.Username, nil
} 
package main

import (
	"errors"
	"strconv"
)

func parseWaitSeconds(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return "", errors.New("waitSeconds must be a non-negative integer")
	}
	return strconv.Itoa(seconds), nil
}

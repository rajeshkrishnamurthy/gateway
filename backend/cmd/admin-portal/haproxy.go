package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
)

func parseHAProxyCSV(data []byte) ([]haproxyFrontend, []haproxyBackend, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return nil, nil, errors.New("empty HAProxy stats")
	}

	header := records[0]
	if len(header) == 0 {
		return nil, nil, errors.New("missing HAProxy header")
	}
	header[0] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header[0]), "#"))
	columns := make(map[string]int)
	for i, name := range header {
		columns[strings.TrimSpace(name)] = i
	}

	get := func(record []string, key string) string {
		idx, ok := columns[key]
		if !ok || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}

	var frontends []haproxyFrontend
	backendOrder := make([]string, 0)
	backendMap := make(map[string]*haproxyBackend)

	for _, record := range records[1:] {
		pxname := get(record, "pxname")
		svname := get(record, "svname")
		status := get(record, "status")
		if pxname == "" || svname == "" {
			continue
		}
		if svname == "FRONTEND" {
			frontends = append(frontends, haproxyFrontend{
				Name:        pxname,
				Status:      status,
				Sessions:    fallbackZero(get(record, "scur")),
				LastChange:  formatSeconds(get(record, "lastchg")),
				StatusClass: statusClass(status),
			})
			continue
		}

		backend, ok := backendMap[pxname]
		if !ok {
			backend = &haproxyBackend{Name: pxname}
			backendMap[pxname] = backend
			backendOrder = append(backendOrder, pxname)
		}

		if svname == "BACKEND" {
			backend.Status = status
			backend.StatusClass = statusClass(status)
			continue
		}

		backend.ServersTotal++
		if isUpStatus(status) {
			backend.ServersUp++
		}
	}

	backends := make([]haproxyBackend, 0, len(backendOrder))
	for _, name := range backendOrder {
		backend := backendMap[name]
		if backend.Status == "" {
			backend.Status = "unknown"
			backend.StatusClass = statusClass(backend.Status)
		}
		backends = append(backends, *backend)
	}

	return frontends, backends, nil
}

func statusClass(status string) string {
	if isUpStatus(status) {
		return "status-up"
	}
	return "status-down"
}

func isUpStatus(status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	return status == "UP" || status == "OPEN"
}

func fallbackZero(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func formatSeconds(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value + "s"
}

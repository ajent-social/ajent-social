package main

import (
	"bufio"
	"os"
	"strings"
)

// Only chat configuration is read from local .env. Never execute dotenv text or
// import unrelated credentials. Production uses Secrets Manager environment.
func loadChatEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || (key != "OPENROUTER_API_KEY" && key != "ASK_MODEL" && key != "ASK_DAILY_LIMIT") {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		os.Setenv(key, value)
	}
}

package logging

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantInfo  bool
		wantDebug bool
	}{
		{name: "default info", level: "", wantInfo: true},
		{name: "info", level: "info", wantInfo: true},
		{name: "debug", level: "debug", wantInfo: true, wantDebug: true},
		{name: "off", level: "off"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := New(test.level, log.New(&output, "", 0))
			logger.Infof("info message")
			logger.Debugf("debug message")
			logger.Errorf("critical error")

			text := output.String()
			if got := strings.Contains(text, "info message"); got != test.wantInfo {
				t.Errorf("info output = %t, want %t; output=%q", got, test.wantInfo, text)
			}
			if got := strings.Contains(text, "debug message"); got != test.wantDebug {
				t.Errorf("debug output = %t, want %t; output=%q", got, test.wantDebug, text)
			}
			if !strings.Contains(text, "critical error") {
				t.Errorf("critical error was suppressed at level %q: %q", test.level, text)
			}
		})
	}
}

func TestErrorLoggerPreservesCriticalMessagesWhenOff(t *testing.T) {
	var output bytes.Buffer
	logger := New("off", log.New(&output, "", 0))
	logger.ErrorLogger().Print("HTTP server failure")
	if !strings.Contains(output.String(), "HTTP server failure") {
		t.Fatalf("ErrorLogger() suppressed a critical message: %q", output.String())
	}
}

package services_test

import (
	"os"
	"strings"
	"testing"
)

func TestServiceAPIErrorHandlingUsesTFUtilHelpers(t *testing.T) {
	files := []string{
		"monitor/error_monitor_resource.go",
		"monitor/metric_monitor_resource.go",
		"notifchan/crud.go",
		"org/org_resource.go",
		"project/project_resource.go",
		"project/project_token_resource.go",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			src := string(srcBytes)

			if strings.Contains(src, `failed", err.Error()`) {
				t.Fatalf("%s still emits raw err.Error() for API failure diagnostics", file)
			}
			if strings.Contains(src, "client.IsNotFound(err)") || strings.Contains(src, "client.IsForbidden(err)") {
				t.Fatalf("%s still uses legacy client status helpers instead of tfutil.IsDeleteGone", file)
			}
		})
	}
}

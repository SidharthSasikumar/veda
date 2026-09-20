package allocdemo

import "fmt"

// Render formats each value as a numbered line. The public format is stable.
func Render(values []string) string {
	out := ""
	for i, value := range values {
		out += fmt.Sprintf("%d:%s\n", i, value)
	}
	return out
}

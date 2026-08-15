package whatsapp

import (
	"regexp"
	"strings"
)

// ParameterPattern matches a Meta template parameter: {{1}}, {{name}},
// {{order_id}}. Meta's template body, header and button URLs all use this
// placeholder syntax.
var ParameterPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// TemplateParamNames returns the parameter names referenced in content, in
// order of first appearance and without duplicates.
//
// This lives here rather than in internal/templateutil because it describes
// Meta's wire format, and a pkg/ package that imports internal/ cannot be used
// by anyone outside this module — which defeats the point of it being in pkg/.
// internal/templateutil now builds its richer resolution helpers on top of this.
func TemplateParamNames(content string) []string {
	matches := ParameterPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(matches))
	var names []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

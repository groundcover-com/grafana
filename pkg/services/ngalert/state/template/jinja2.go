package template

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// NestedLabels is a map[string]interface{} that supports both bracket notation
// (flat key lookup) and dot notation (nested traversal) for keys containing dots.
// It preserves the original Labels.String() formatting for direct {{ labels }} interpolation.
type NestedLabels map[string]interface{}

func (l NestedLabels) String() string {
	if len(l) == 0 {
		return ""
	}

	// Render scalar leaves only: plain string labels and labelMap nodes that carry a
	// scalar of their own (a label that is also a namespace). Pure namespace nodes are skipped.
	keys := make([]string, 0, len(l))
	for k, v := range l {
		switch val := v.(type) {
		case string:
			keys = append(keys, k)
		case labelMap:
			if _, ok := val.raw(); ok {
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		// %s uses labelMap.String() (its scalar) for nodes, or the plain string value.
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, l[k]))
	}
	return strings.Join(pairs, ", ")
}

// rawValueKey is the sentinel map key under which a labelMap stores its own scalar
// value. The NUL byte guarantees it can never collide with a real label segment
// (label keys never contain NUL bytes).
const rawValueKey = "\x00raw"

// labelMap represents a label that is simultaneously a scalar value and a namespace.
// It is map-kinded so pongo2 resolves nested keys ({{ labels.error.message }}), and
// implements fmt.Stringer so direct output ({{ labels.error }}) renders the node's own
// scalar value instead of a Go map dump. This lets a flat label like "error" coexist
// with a dotted label "error.message" without the dot-access collision that a plain
// string value would otherwise cause.
type labelMap map[string]interface{}

// String renders the node's own scalar value, or empty if it is a pure namespace.
func (m labelMap) String() string {
	raw, _ := m.raw()
	return raw
}

// raw reports the node's own scalar value and whether it has one.
func (m labelMap) raw() (string, bool) {
	v, ok := m[rawValueKey].(string)
	return v, ok
}

// BuildNestedLabels converts a flat Labels map into a NestedLabels that supports both
// bracket notation (labels["a.b"]) and dot notation (labels.a.b) for keys containing
// dots. When a key is both a value and a namespace prefix of another key (e.g. "error"
// alongside "error.message"), the value is preserved as a labelMap node so that both
// {{ labels.error }} and {{ labels.error.message }} resolve.
//
// For {"http.route": "/api", "env": "prod"} the result is:
//
//	{
//	  "http.route": "/api",                // flat key preserved for labels["http.route"]
//	  "http": labelMap{"route": "/api"},   // nested node for labels.http.route
//	  "env": "prod",                       // simple key, works both ways
//	}
func BuildNestedLabels(labels Labels) NestedLabels {
	if len(labels) == 0 {
		return nil
	}

	result := make(NestedLabels, len(labels)*2)

	// Seed every original flat key so bracket notation (labels["a.b"]) always works.
	for k, v := range labels {
		result[k] = v
	}

	// Build nested labelMap structure for dotted keys. Process keys with fewer
	// segments first so a shorter key ("a.b") settles before a deeper one ("a.b.c")
	// descends through it, keeping construction deterministic.
	type dottedKey struct {
		key   string
		parts []string
	}
	var dottedKeys []dottedKey
	for k := range labels {
		if parts := strings.Split(k, "."); len(parts) > 1 {
			dottedKeys = append(dottedKeys, dottedKey{key: k, parts: parts})
		}
	}
	sort.Slice(dottedKeys, func(i, j int) bool {
		if len(dottedKeys[i].parts) != len(dottedKeys[j].parts) {
			return len(dottedKeys[i].parts) < len(dottedKeys[j].parts)
		}
		return dottedKeys[i].key < dottedKeys[j].key
	})

	for _, dk := range dottedKeys {
		var current map[string]interface{} = result
		// Walk/create intermediate nodes, promoting any colliding scalar into a node.
		for _, part := range dk.parts[:len(dk.parts)-1] {
			current = promoteToNode(current, part)
		}
		// Place the leaf value. If a deeper key already created a node here, attach
		// this key's scalar to that node rather than overwriting it.
		leaf := dk.parts[len(dk.parts)-1]
		if node, ok := current[leaf].(labelMap); ok {
			node[rawValueKey] = labels[dk.key]
		} else {
			current[leaf] = labels[dk.key]
		}
	}

	return result
}

// promoteToNode returns the labelMap stored at m[key], creating it if absent and
// promoting a pre-existing scalar value into the node's scalar slot so it survives.
func promoteToNode(m map[string]interface{}, key string) labelMap {
	switch existing := m[key].(type) {
	case labelMap:
		return existing
	case nil:
		node := labelMap{}
		m[key] = node
		return node
	default: // scalar — a flat label that is also a namespace prefix
		node := labelMap{rawValueKey: existing}
		m[key] = node
		return node
	}
}

func init() {
	// Disable auto-escaping globally for pongo2.
	// Auto-escaping is disabled because summary templates are used for plain text output,
	// not HTML rendering, so we want raw string values without HTML entity encoding.
	pongo2.SetAutoescape(false)
}

// maxTemplateSize bounds user-provided summary templates (10 KB).
const maxTemplateSize = 10 * 1024

// bannedTags are pongo2 tags capable of reading local files. They must never be
// available in user-provided summary templates.
var bannedTags = []string{
	"include", // {% include "file" %} — loads and renders a template file
	"ssi",     // {% ssi "file" %}     — server-side include, reads file content
	"extends", // {% extends "file" %} — inherits from a parent template file
	"import",  // {% import "file" %}  — imports macros from a template file
}

// denyLoader is a pongo2.TemplateLoader that rejects all filesystem access.
// pongo2.NewSet requires a loader, so we supply one that always errors.
type denyLoader struct{}

func (denyLoader) Abs(base, name string) string { return name }
func (denyLoader) Get(string) (io.Reader, error) {
	return nil, fmt.Errorf("template loading from filesystem is disabled")
}

// sandboxSet is a pongo2 TemplateSet with file-reading tags banned and filesystem
// access denied, so a summary template cannot exfiltrate local files.
var sandboxSet = newSandboxSet()

func newSandboxSet() *pongo2.TemplateSet {
	set := pongo2.NewSet("ngalert-jinja2-sandbox", denyLoader{})
	for _, tag := range bannedTags {
		if err := set.BanTag(tag); err != nil {
			panic(fmt.Sprintf("jinja2 sandbox: failed to ban tag %q: %v", tag, err))
		}
	}
	return set
}

// sandboxedFromString parses a template through the sandboxed set, rejecting
// over-sized templates and any use of the banned file-reading tags.
func sandboxedFromString(tmpl string) (*pongo2.Template, error) {
	if len(tmpl) > maxTemplateSize {
		return nil, fmt.Errorf("template exceeds maximum size of %d bytes", maxTemplateSize)
	}
	return sandboxSet.FromString(tmpl)
}

// safeExecute runs tpl.Execute but converts a pongo2 panic into an error. pongo2
// panics (rather than returning an error) on some malformed expressions — notably an
// unquoted bracket subscript like {{ labels[key] }} — which would otherwise crash the
// caller's goroutine. Recovering keeps summary expansion fail-safe.
func safeExecute(tpl *pongo2.Template, ctx pongo2.Context) (result string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			result = ""
			err = fmt.Errorf("template execution panicked: %v "+
				"(a likely cause is unquoted bracket access such as labels[key]; quote the key: labels[\"key\"])", rec)
		}
	}()
	return tpl.Execute(ctx)
}

type SummaryContext struct {
	MonitorName string
	Severity    string
	Labels      Labels
	Fingerprint string
	Value       float64
	Threshold   float64
	State       string
	Query       string
	Creator     string
}

// ExpandJinja2Summary expands a Jinja2-style summary template with the given context.
// Supports variable interpolation using {{ variable }} syntax.
// Available variables: monitor_name, severity, labels, fingerprint, value, threshold, state, query, creator
// To support legacy monitors, we also inject all the labels as alert.labels, monitor_name as alert.alertname and fingerprint as alert.fingerprint.
func ExpandJinja2Summary(tmpl string, ctx SummaryContext) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	nestedLabels := BuildNestedLabels(ctx.Labels)

	pongoCtx := pongo2.Context{
		"monitor_name": ctx.MonitorName,
		"severity":     ctx.Severity,
		"labels":       nestedLabels,
		"value":        ctx.Value,
		"threshold":    ctx.Threshold,
		"state":        ctx.State,
		"query":        ctx.Query,
		"creator":      ctx.Creator,
		"fingerprint":  ctx.Fingerprint,
		// Legacy support for alert. prefix (Keep workflows)
		"alert": map[string]any{
			"labels":      nestedLabels,
			"fingerprint": ctx.Fingerprint,
			"alertname":   ctx.MonitorName,
		},
	}

	tpl, err := sandboxedFromString(tmpl)
	if err != nil {
		return "", ExpandError{Tmpl: tmpl, Err: err}
	}

	result, err := safeExecute(tpl, pongoCtx)
	if err != nil {
		return "", ExpandError{Tmpl: tmpl, Err: err}
	}

	return result, nil
}

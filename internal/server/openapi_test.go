package server

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	specPath   = regexp.MustCompile(`^  (/\S*):$`)
	specMethod = regexp.MustCompile(`^    (get|put|post|delete|patch):$`)
)

// specOperations reads "METHOD /path" pairs from the paths section of
// docs/openapi.yaml (two-space indentation for paths, four for methods).
func specOperations(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]bool{}
	inPaths, current := false, ""
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && line != "" && line[0] != ' ':
			inPaths = false
		case !inPaths:
		case specPath.MatchString(line):
			current = specPath.FindStringSubmatch(line)[1]
		case specMethod.MatchString(line):
			ops[strings.ToUpper(specMethod.FindStringSubmatch(line)[1])+" "+current] = true
		}
	}
	return ops
}

// TestOpenAPISpecMatchesRoutes keeps docs/openapi.yaml and the route table
// in sync: every route is documented and every documented operation exists.
func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	s := newTestServer(t, testFS())
	routes := map[string]bool{}
	for _, rt := range s.routes() {
		routes[rt.method+" "+strings.ReplaceAll(rt.pattern, "...", "")] = true
	}
	spec := specOperations(t)
	if len(spec) == 0 {
		t.Fatal("no operations found in docs/openapi.yaml")
	}
	for _, op := range sortedKeys(routes) {
		if !spec[op] {
			t.Errorf("route %s is missing in docs/openapi.yaml", op)
		}
	}
	for _, op := range sortedKeys(spec) {
		if !routes[op] {
			t.Errorf("docs/openapi.yaml documents %s, but the server has no such route", op)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

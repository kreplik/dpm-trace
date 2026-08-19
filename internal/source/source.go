// Package source maps failures back to Daml source: file, line, column and a
// caret. Ports SourceIndex and the diagnostic renderers from cli.py.
//
// Precedence rule from AGENTS.md, preserved here: prefer damlc inspect plus
// project metadata, with local source text matching only as a fallback. That is
// why find_failure_text consults the inspected modules first and only then
// falls back to scanning every loaded file.
package source

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// Location is a resolved position in a Daml source file.
type Location struct {
	Path      string
	Line      int
	Label     string
	Column    int
	EndLine   int
	EndColumn int
}

// Index holds the Daml sources a diagnostic can be resolved against.
type Index struct {
	Roots []string

	files        map[string][]string // path -> lines
	fileModules  map[string]string   // path -> module name
	moduleFiles  map[string][]string // module name -> paths
	inspectModul map[string][]string // module name -> lines from damlc inspect
	templates    map[string]Location // "<pkg>:<module>:<entity>" -> definition
	choices      map[string]Location // "<pkg>:<module>:<entity>.<choice>"
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{
		files:        map[string][]string{},
		fileModules:  map[string]string{},
		moduleFiles:  map[string][]string{},
		inspectModul: map[string][]string{},
		templates:    map[string]Location{},
		choices:      map[string]Location{},
	}
}

// HasSources reports whether anything was loaded.
func (ix *Index) HasSources() bool {
	if len(ix.templates) > 0 || len(ix.choices) > 0 {
		return true
	}
	return len(ix.files) > 0
}

// Lines returns the loaded content of a file.
func (ix *Index) Lines(path string) ([]string, bool) {
	lines, ok := ix.files[path]
	return lines, ok
}

var sourceFieldPattern = regexp.MustCompile(`^\s*source\s*:\s*(.+?)\s*$`)

// LoadDamlYAML reads the source roots declared by a daml.yaml and loads them.
// Ports SourceIndex._load_daml_yaml. A missing or unreadable file is ignored:
// source mapping is best-effort and must never fail a trace.
func (ix *Index) LoadDamlYAML(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var values []string
	for _, line := range strings.Split(string(data), "\n") {
		if match := sourceFieldPattern.FindStringSubmatch(line); match != nil {
			values = append(values, stripYAMLScalar(match[1]))
		}
	}
	if len(values) == 0 {
		values = []string{"."}
	}
	for _, value := range values {
		root := value
		if !filepath.IsAbs(root) {
			root = filepath.Join(filepath.Dir(path), root)
		}
		ix.LoadSourceRoot(root)
	}
}

// LoadSourceRoot loads a single .daml file or every .daml file beneath a
// directory. Ports SourceIndex._load_source_root.
func (ix *Index) LoadSourceRoot(root string) {
	resolved, err := filepath.Abs(root)
	if err != nil {
		return
	}
	// A root that does not exist is still recorded: the summary lists what was
	// asked for, and silently dropping it hides a typo.
	info, err := os.Stat(resolved)
	if err != nil {
		if !containsString(ix.Roots, resolved) {
			ix.Roots = append(ix.Roots, resolved)
		}
		return
	}

	var candidates []string
	marker := resolved
	if info.IsDir() {
		_ = filepath.WalkDir(resolved, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(path, ".daml") {
				candidates = append(candidates, path)
			}
			return nil
		})
		sort.Strings(candidates)
	} else {
		candidates = []string{resolved}
		marker = filepath.Dir(resolved)
	}

	if !containsString(ix.Roots, marker) {
		ix.Roots = append(ix.Roots, marker)
	}
	for _, candidate := range candidates {
		ix.loadSourceFile(candidate)
	}
}

var modulePattern = regexp.MustCompile(`^\s*module\s+([A-Za-z0-9_.']+)\s+where\b`)

func (ix *Index) loadSourceFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := splitLines(string(data))
	ix.files[path] = lines

	if module := damlModuleName(lines); module != "" {
		ix.fileModules[path] = module
		if !containsString(ix.moduleFiles[module], path) {
			ix.moduleFiles[module] = append(ix.moduleFiles[module], path)
		}
	}
}

// FindFailureText locates a failure message in the loaded sources, preferring
// modules that damlc inspect confirmed contain it.
// Ports SourceIndex.find_failure_text.
func (ix *Index) FindFailureText(text string, limit int) []Location {
	if text == "" {
		return nil
	}
	if modules := ix.inspectModulesContaining(text); len(modules) > 0 {
		var paths []string
		for _, module := range modules {
			paths = append(paths, ix.moduleFiles[module]...)
		}
		if len(paths) > 0 {
			shown := modules
			if len(shown) > 3 {
				shown = shown[:3]
			}
			return ix.findText(text, "damlc inspect: "+strings.Join(shown, ", "), paths, limit)
		}
	}
	return ix.findText(text, "local source", nil, limit)
}

func (ix *Index) inspectModulesContaining(text string) []string {
	if text == "" {
		return nil
	}
	modules := make([]string, 0, len(ix.inspectModul))
	for module := range ix.inspectModul {
		modules = append(modules, module)
	}
	sort.Strings(modules)

	var result []string
	for _, module := range modules {
		for _, line := range ix.inspectModul[module] {
			if strings.Contains(line, text) {
				result = append(result, module)
				break
			}
		}
	}
	return result
}

func (ix *Index) findText(text, label string, paths []string, limit int) []Location {
	if text == "" {
		return nil
	}
	search := paths
	if search == nil {
		search = make([]string, 0, len(ix.files))
		for path := range ix.files {
			search = append(search, path)
		}
		sort.Strings(search)
	}

	var result []Location
	for _, path := range search {
		lines, ok := ix.files[path]
		if !ok {
			continue
		}
		for i, line := range lines {
			offset := strings.Index(line, text)
			if offset < 0 {
				continue
			}
			// Columns count characters, not bytes: str.find does, editors do,
			// and a non-ASCII string literal earlier on the line (common in
			// assertMsg) would otherwise shift the reported column and the
			// caret to the right.
			column := utf8.RuneCountInString(line[:offset])
			result = append(result, Location{Path: path, Line: i + 1, Label: label, Column: column + 1})
			if len(result) >= limit {
				return result
			}
		}
	}
	return result
}

// Snippet renders the matched line with surrounding context and a caret.
// Ports SourceIndex.snippet.
func (ix *Index) Snippet(loc Location, radius int) string {
	lines, ok := ix.files[loc.Path]
	if !ok || len(lines) == 0 {
		return FormatPath(loc)
	}
	start := loc.Line - radius
	if start < 1 {
		start = 1
	}
	end := loc.Line + radius
	if end > len(lines) {
		end = len(lines)
	}

	width := len(fmt.Sprint(end))
	var rendered []string
	for lineNo := start; lineNo <= end; lineNo++ {
		marker := " "
		if lineNo == loc.Line {
			marker = ">"
		}
		prefix := fmt.Sprintf("%s %*d | ", marker, width, lineNo)
		rendered = append(rendered, prefix+lines[lineNo-1])
		if lineNo == loc.Line && loc.Column > 1 {
			// Pad in characters for the same reason the column is counted in
			// characters; len() would drift on a multibyte line.
			rendered = append(rendered, strings.Repeat(" ", utf8.RuneCountInString(prefix)+loc.Column-1)+"^")
		}
	}
	return FormatPath(loc) + "\n" + strings.Join(rendered, "\n")
}

// FormatPath renders "path:line", adding the column only when it is meaningful.
// A column of 1 usually means "start of line" rather than a resolved position.
// Ports format_source_path.
func FormatPath(loc Location) string {
	if loc.Column > 1 {
		return fmt.Sprintf("%s:%d:%d", loc.Path, loc.Line, loc.Column)
	}
	return fmt.Sprintf("%s:%d", loc.Path, loc.Line)
}

var needlePatterns = []*regexp.Regexp{
	regexp.MustCompile(`"([^"]{4,})"`),
	regexp.MustCompile(`'([^']{4,})'`),
	regexp.MustCompile("`([^`]{4,})`"),
}

// Needles extracts candidate search strings from a failure message, most
// specific first. Ports completion_source_needles.
func Needles(message string) []string {
	var candidates []string
	for _, pattern := range needlePatterns {
		for _, match := range pattern.FindAllStringSubmatch(message, -1) {
			candidates = append(candidates, strings.TrimSpace(match[1]))
		}
	}
	for _, separator := range []string{":", "because", "failed with"} {
		if index := strings.LastIndex(message, separator); index >= 0 {
			candidates = append(candidates, strings.TrimSpace(message[index+len(separator):]))
		}
	}
	candidates = append(candidates, strings.TrimSpace(message))

	var cleaned []string
	for _, candidate := range candidates {
		candidate = strings.Trim(candidate, " .\n\r\t")
		if utf8.RuneCountInString(candidate) < 4 || containsString(cleaned, candidate) {
			continue
		}
		cleaned = append(cleaned, candidate)
	}
	return cleaned
}

func damlModuleName(lines []string) string {
	limit := len(lines)
	if limit > 50 {
		limit = 50
	}
	for _, line := range lines[:limit] {
		if match := modulePattern.FindStringSubmatch(line); match != nil {
			return match[1]
		}
	}
	return ""
}

func stripYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		// Either quote character on either end, matched or not: strip_yaml_scalar
		// tests startswith/endswith independently.
		isQuote := func(c byte) bool { return c == '"' || c == '\'' }
		if isQuote(value[0]) && isQuote(value[len(value)-1]) {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// splitLines mirrors Python's str.splitlines(): a trailing newline does not
// produce a final empty element.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// EntityContaining returns the kind and label of the innermost choice or
// template whose definition starts at or before line, in the same file.
//
// It is what turns a bare "Asset.daml:54" into "Asset.daml:54 in choice
// Asset.Withdraw" -- the difference between a line number and an explanation.
// Ports SourceIndex.entity_containing.
func (ix *Index) EntityContaining(path string, line int) (kind, label string, ok bool) {
	best := 0
	consider := func(loc Location, entity string) {
		if loc.Path != path || loc.Line <= 0 || loc.Line > line {
			return
		}
		if best == 0 || loc.Line > best {
			best, kind, label, ok = loc.Line, entity, loc.Label, true
		}
	}
	for _, loc := range ix.choices {
		consider(loc, "choice")
	}
	for _, loc := range ix.templates {
		consider(loc, "template")
	}
	return kind, label, ok
}

// ModuleFiles returns the loaded files declaring a module, in load order.
func (ix *Index) ModuleFiles(module string) []string {
	return ix.moduleFiles[module]
}

// LoadDARInspect records the module text `damlc inspect` reports for a DAR.
//
// This is what makes a failure match trustworthy: a needle found in inspected
// module text is confirmed present in the compiled package, where a match in a
// local file is only a guess about the source that produced it. Failures here
// are silent by design -- source mapping is best-effort and must never fail a
// trace. Ports SourceIndex._load_dar_inspect.
func (ix *Index) LoadDARInspect(darPath, damlc string) {
	if _, err := os.Stat(darPath); err != nil {
		return
	}
	command := DamlcInspectCommand(damlc, darPath)

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = childEnv()
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return
	}

	current := ""
	for _, line := range strings.Split(stdout.String(), "\n") {
		if match := inspectModulePattern.FindStringSubmatch(line); match != nil {
			current = match[1]
			if _, seen := ix.inspectModul[current]; !seen {
				ix.inspectModul[current] = []string{}
			}
			continue
		}
		if current != "" {
			ix.inspectModul[current] = append(ix.inspectModul[current], line)
		}
	}
}

var inspectModulePattern = regexp.MustCompile(`^module\s+([A-Za-z0-9_.']+)\s+where\b`)

// DamlcInspectCommand builds the inspect invocation, accepting either damlc
// directly or the daml assistant. Ports damlc_inspect_command.
func DamlcInspectCommand(damlc, darPath string) []string {
	executable := expandUser(damlc)
	if filepath.Base(executable) == "damlc" {
		return []string{executable, "inspect", darPath, "--detail", "2"}
	}
	return []string{executable, "damlc", "inspect", darPath, "--detail", "2"}
}

func expandUser(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	// "~name/x" is that user's home, not a directory under ours. An unknown
	// user is left alone rather than silently rewritten.
	if len(path) > 1 && path[1] != '/' {
		name := path[1:]
		rest := ""
		if slash := strings.Index(name, "/"); slash >= 0 {
			name, rest = name[:slash], name[slash+1:]
		}
		other, err := user.Lookup(name)
		if err != nil {
			return path
		}
		return filepath.Join(other.HomeDir, rest)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// childEnv mirrors daml_child_env for the inspect subprocess: DPM_RESOLUTION_FILE
// is dropped so damlc resolves the target package rather than this component's
// plugin context, and a UTF-8 locale is forced.
func childEnv() []string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, found := strings.Cut(entry, "="); found {
			env[key] = value
		}
	}
	delete(env, "DPM_RESOLUTION_FILE")

	ctype := env["LC_ALL"]
	if ctype == "" {
		ctype = env["LC_CTYPE"]
	}
	if ctype == "" {
		ctype = env["LANG"]
	}
	if !strings.Contains(strings.ToLower(ctype), "utf") {
		env["LANG"] = "C.UTF-8"
		env["LC_ALL"] = "C.UTF-8"
	}

	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

// LocationForEvent resolves an event to where its template or choice is
// defined. Only debug info populates these tables, so without --debug-info this
// reports no location, matching Python. Ports SourceIndex.location_for_event.
func (ix *Index) LocationForEvent(ev *model.Event) *Location {
	packageID, module, entity, ok := ParseTemplateRef(ev.Template)
	if !ok {
		return nil
	}
	key := packageID + ":" + module + ":" + entity
	if ev.Choice != "" {
		if loc, found := ix.choices[key+"."+ev.Choice]; found {
			return &loc
		}
		return nil
	}
	if loc, found := ix.templates[key]; found {
		return &loc
	}
	return nil
}

// LoadDebugInfo reads a daml-debug-info/v1 file: a package id, a source root
// and per-file entity positions.
//
// This is what makes `s`/`source` and per-event source lines work -- it maps a
// template or choice to where it is defined, which neither local text matching
// nor damlc inspect can do. Ports SourceIndex._load_debug_info.
func (ix *Index) LoadDebugInfo(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	doc, err := model.Decode(data)
	if err != nil {
		return
	}
	packageID := model.ObjectString(doc, "packageId")
	if packageID == "" {
		return
	}

	sourceRoot := model.ObjectString(doc, "sourceRoot")
	if sourceRoot == "" {
		sourceRoot = filepath.Dir(path)
	}
	sourceRoot = expandUser(sourceRoot)
	if !filepath.IsAbs(sourceRoot) {
		if resolved, err := filepath.Abs(filepath.Join(filepath.Dir(path), sourceRoot)); err == nil {
			sourceRoot = resolved
		}
	}
	if !containsString(ix.Roots, sourceRoot) {
		ix.Roots = append(ix.Roots, sourceRoot)
	}

	files, _ := model.ObjectValue(doc, "files").([]any)
	for _, item := range files {
		fileInfo, ok := item.(*model.Object)
		if !ok {
			continue
		}
		rawPath := model.ObjectString(fileInfo, "path")
		if rawPath == "" {
			continue
		}
		filePath := expandUser(rawPath)
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(sourceRoot, filePath)
		}
		if content, err := os.ReadFile(filePath); err == nil {
			ix.files[filePath] = splitLines(string(content))
		}

		entities, _ := model.ObjectValue(fileInfo, "entities").([]any)
		for _, raw := range entities {
			entity, ok := raw.(*model.Object)
			if !ok {
				continue
			}
			name := model.ObjectString(entity, "qualifiedName")
			kind := model.ObjectString(entity, "kind")
			line := model.ObjectInt(entity, "startLine")
			if name == "" || kind == "" || line == nil {
				continue
			}
			key := packageID + ":" + name
			loc := Location{Path: filePath, Line: int(*line), Label: name, Column: 1}
			switch kind {
			case "template":
				ix.templates[key] = loc
			case "choice":
				ix.choices[key] = loc
			}
		}
	}
}

// ParseTemplateRef splits a package:module:entity template reference.
// Ports parse_template_ref.
func ParseTemplateRef(template string) (packageID, module, entity string, ok bool) {
	parts := strings.Split(template, ":")
	if template == "" || len(parts) < 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

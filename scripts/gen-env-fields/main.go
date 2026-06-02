// gen-env-fields fetches Ping Identity product README files and prints suggested
// Go struct field declarations for each product's Config type.
//
// Fields already present in the types file are annotated EXISTING; new fields
// are annotated NEW so they can be pasted directly into pingenvironment_types.go.
//
// Usage:
//
//	go run ./scripts/gen-env-fields [flags] <base-raw-url> [product ...]
//
// Flags:
//
//	-types     path to pingenvironment_types.go (default: api/v1alpha1/pingenvironment_types.go)
//	-new-only  suppress EXISTING fields; print only NEW fields
//
// Examples:
//
//	# All products against the latest commit
//	go run ./scripts/gen-env-fields \
//	  https://raw.githubusercontent.com/pingidentity/pingidentity-devops-getting-started/master/docs/docker-images
//
//	# Pin to a specific commit, filter to one product
//	go run ./scripts/gen-env-fields \
//	  https://raw.githubusercontent.com/pingidentity/pingidentity-devops-getting-started/1b1c624e/docs/docker-images \
//	  pingfederate
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// ─────────────────────────── product registry ────────────────────────────────

// product maps a Ping Identity image to its Go Config struct and any
// product-specific env-var prefixes to strip when deriving Go field names.
type product struct {
	path       string   // subdirectory name under base URL (e.g. "pingfederate")
	structName string   // Go struct name (e.g. "PingFederateConfig")
	prefixes   []string // env-var prefixes stripped before name conversion
}

var registry = []product{
	{path: "pingfederate", structName: "PingFederateConfig", prefixes: []string{"PF_"}},
	{path: "pingdirectory", structName: "PingDirectoryConfig", prefixes: []string{"PD_"}},
	{path: "pingaccess", structName: "PingAccessConfig", prefixes: []string{"PA_"}},
	{path: "pingauthorize", structName: "PingAuthorizeConfig", prefixes: nil},
	{path: "pingauthorizepap", structName: "PingAuthorizePAPConfig", prefixes: []string{"PING_"}},
	{path: "pingdatasync", structName: "PingDataSyncConfig", prefixes: []string{"PD_"}},
	{path: "pingdirectoryproxy", structName: "PingDirectoryProxyConfig", prefixes: []string{"PD_"}},
	{path: "pingdataconsole", structName: "PingDataConsoleSpec", prefixes: nil},
}

// ─────────────────────────── skip lists ──────────────────────────────────────

// skipPrefixes: any env var whose name starts with one of these is a container
// infrastructure or PingBase toolkit var, never surfaced as a CRD field.
var skipPrefixes = []string{
	"SERVER_PROFILE_",  // handled via ServerProfileSpec / ServerProfileLayerSpec
	"PING_IDENTITY_",   // devops licensing
	"STARTUP_",         // container entrypoint internals
	"LICENSE_",         // licensing internals
	"IMAGE_",           // build metadata
	"PING_PRODUCT",     // product metadata
	"SHIM",
	"DATE",
	"ENV",
	"PS1",
	"PATH",
}

// skipSuffixes: env vars whose name ends with one of these are always internal.
var skipSuffixes = []string{
	"_PRIVATE_HOSTNAME", // cluster-internal DNS, not user-facing
}

// skipExact: exact env var names always skipped.
var skipExact = map[string]bool{
	// paths / dirs
	"BASE": true, "OUT_DIR": true, "IN_DIR": true, "BAK_DIR": true,
	"LOG_DIR": true, "LOGS_DIR": true, "TMP_DIR": true,
	"STAGING_DIR": true, "HOOKS_DIR": true, "SECRETS_DIR": true,
	"SERVER_ROOT_DIR": true, "SERVER_BITS_DIR": true,
	"SERVER_PROFILE_DIR": true, "TOPOLOGY_FILE": true,
	"CONTAINER_ENV": true, "STAGING_MANIFEST": true,
	"BULK_CONFIG_DIR": true, "BULK_CONFIG_FILE": true,

	// runtime / shell
	"JAVA_HOME": true, "JVM_TUNING": true,
	"TAIL_LOG_FILES": true, "ROOT_USER_DN": true,
	"CLUSTER_BIND_ADDRESS": true, // auto-set by operator when OPERATIONAL_MODE != STANDALONE
	"CLEAN_STAGING_DIR": true,
	"SECURITY_CHECKS_STRICT": true, "SECURITY_CHECKS_FILENAME": true,
	"UNSAFE_CONTINUE_ON_ERROR": true,
	"SERVER_PROFILE_URL_REDACT": true,
	"SHOW_LIBS_VER": true, "SHOW_LIBS_VER_PRE_PATCH": true,
	"DOLLAR": true,

	// build metadata
	"PING_PRODUCT_VERSION": true,

	// handled by typed Kubernetes Secret refs in the operator (not plain env vars):
	"ROOT_USER_PASSWORD_FILE": true,
	"ADMIN_USER_PASSWORD_FILE": true,
	"ENCRYPTION_PASSWORD_FILE": true,
	"KEYSTORE_FILE": true, "KEYSTORE_PIN_FILE": true,
	"TRUSTSTORE_FILE": true, "TRUSTSTORE_PIN_FILE": true,
	"PF_LDAP_PASSWORD": true, // credential — handled via LDAPSecretRef
	"PING_IDENTITY_ACCEPT_EULA": true,

	// other-product hostnames that appear in cross-product READMEs
	"PAZ_ENGINE_PUBLIC_HOSTNAME": true, "PAZ_ENGINE_PRIVATE_HOSTNAME": true,
	"PAZP_ENGINE_PUBLIC_HOSTNAME": true, "PAZP_ENGINE_PRIVATE_HOSTNAME": true,
	"PA_ENGINE_PUBLIC_HOSTNAME": true, "PA_ENGINE_PRIVATE_HOSTNAME": true,
	"PA_ADMIN_PUBLIC_HOSTNAME": true, "PA_ADMIN_PRIVATE_HOSTNAME": true,
	"PD_ENGINE_PUBLIC_HOSTNAME": true,
	"PDP_ENGINE_PUBLIC_HOSTNAME": true, "PDS_ENGINE_PUBLIC_HOSTNAME": true,

	// UNBOUNDID_SKIP_START_PRECHECK_NODETACH is hardcoded true by the operator
	"UNBOUNDID_SKIP_START_PRECHECK_NODETACH": true,

	// JMX monitoring port — not surfaced via CRD
	"JMX_PORT": true,

	// Orchestration type — set by the container runtime, not the operator
	"ORCHESTRATION_TYPE": true,
}

// ─────────────────────────── type-inference tables ───────────────────────────

// acronyms are kept ALL_CAPS in Go field names.
var acronyms = map[string]bool{
	"LDAP": true, "LDAPS": true, "TLS": true, "SSL": true,
	"HTTP": true, "HTTPS": true, "API": true, "URL": true,
	"DNS": true, "JVM": true, "RAM": true, "ID": true,
	"FIPS": true, "BC": true, "HSM": true, "DB": true,
	"OIDC": true, "JWT": true, "PEM": true, "CA": true,
	"AD": true, "PAP": true, "PAZ": true, "EULA": true,
}

// intSuffixes: env var names ending with these → int32 when there is no
// non-numeric default value to override the inference.
var intSuffixes = []string{
	"_PORT", "_SECONDS", "_TIMEOUT", "_PERIOD",
	"_NUMBER", "_USERS", "_NODE_ID", "_MIN", "_MAX",
}

// boolContains: env var names containing these substrings → bool.
// Checked AFTER the default-value check so a numeric default always wins.
var boolContains = []string{
	"FIPS_MODE_ON", "_DEBUG", "_MODE_ON", "_ONLY", "_HYBRID",
	"SKIP_WAIT_", "FAIL_ON_", "REBUILD_ON_RESTART", "FORCE_DATA_",
	"PARALLEL_POD", "CREATE_INITIAL_ADMIN", "ENABLE_AUTOMATIC",
	"JOIN_PD_TOPOLOGY",
}

// ─────────────────────────── main ────────────────────────────────────────────

func main() {
	typesFile := flag.String("types", "api/v1alpha1/pingenvironment_types.go",
		"path to pingenvironment_types.go used for NEW/EXISTING annotation (empty to skip)")
	newOnly := flag.Bool("new-only", false, "suppress EXISTING fields; print only NEW fields")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gen-env-fields [flags] <base-raw-url> [product ...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "example:")
		fmt.Fprintln(os.Stderr, "  go run ./scripts/gen-env-fields \\")
		fmt.Fprintln(os.Stderr, "    https://raw.githubusercontent.com/pingidentity/pingidentity-devops-getting-started/master/docs/docker-images")
		os.Exit(1)
	}

	baseURL := strings.TrimSuffix(args[0], "/")
	filter := map[string]bool{}
	for _, p := range args[1:] {
		filter[p] = true
	}

	existing := map[string]bool{}
	if *typesFile != "" {
		existing = loadExistingJSONTags(*typesFile)
	}

	for _, prod := range registry {
		if len(filter) > 0 && !filter[prod.path] {
			continue
		}

		url := baseURL + "/" + prod.path + "/README.md"
		fmt.Printf("// ══════════════════════════════════════════════════════════\n")
		fmt.Printf("// %s\n", prod.structName)
		fmt.Printf("// Source: %s\n", url)
		fmt.Printf("// ══════════════════════════════════════════════════════════\n\n")

		content, err := fetchURL(url)
		if err != nil {
			fmt.Printf("// ERROR: %v\n\n", err)
			continue
		}

		vars := parseEnvVars(content)
		if len(vars) == 0 {
			fmt.Printf("// No environment-variable table found in README.\n\n")
			continue
		}

		newCount := 0
		for _, ev := range vars {
			if shouldSkip(ev.Name) {
				continue
			}

			f, jsonTag := deriveField(ev, prod.prefixes)
			isNew := !existing[jsonTag]
			if *newOnly && !isNew {
				continue
			}
			if isNew {
				newCount++
			}

			status := "EXISTING"
			if isNew {
				status = "NEW     "
			}

			comment := buildComment(f.name, ev)
			fmt.Printf("\t// %s %s\n", status, comment)
			fmt.Printf("\t%s %s `json:\"%s,omitempty\"`\n", f.name, f.typ, jsonTag)
		}
		fmt.Printf("\n// %d new field(s) for %s\n\n", newCount, prod.structName)
	}
}

// ─────────────────────────── env var parsing ─────────────────────────────────

type envVar struct {
	Name        string
	Default     string
	Description string
}

// parseEnvVars locates the "## Environment Variables" section of a README and
// returns all env vars extracted from its markdown table.
//
// The Ping Identity README format uses plain-text (unquoted) variable names:
//
//	| ENV Variable  | Default     | Description
//	| ------------: | ----------- | --------------------------------
//	| PF_ENGINE_PORT  | 9031  | Port for the engine ...  |
//
// as well as backtick-quoted names used in other products:
//
//	| `PF_ENGINE_PORT` | `9031` | Port for the engine ... |
func parseEnvVars(content string) []envVar {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	inSection := false
	inTable := false
	headerSeen := false

	colVar, colDef, colDesc := -1, -1, -1

	backtickWord := regexp.MustCompile("`([^`]+)`")
	defaultInDesc := regexp.MustCompile(`(?i)default[s]?:\s*` + "`([^`]*)`")
	sectionRe := regexp.MustCompile(`(?i)^#{1,4}\s+environment\s+variables`)
	nextSectionRe := regexp.MustCompile(`^#{1,3} `)

	var vars []envVar

	for _, raw := range lines {
		line := strings.TrimSpace(raw)

		// Enter environment variables section
		if sectionRe.MatchString(line) {
			inSection = true
			inTable = false
			headerSeen = false
			colVar, colDef, colDesc = -1, -1, -1
			continue
		}

		// Exit on next same/higher-level section heading
		if inSection && nextSectionRe.MatchString(line) && !sectionRe.MatchString(line) {
			break
		}

		if !inSection {
			continue
		}

		if !strings.HasPrefix(line, "|") {
			if inTable {
				inTable = false
			}
			continue
		}

		cols := splitRow(line)

		// ── Parse table header ────────────────────────────────────────────
		if !inTable {
			colVar, colDef, colDesc = -1, -1, -1
			for i, col := range cols {
				lower := strings.ToLower(col)
				switch {
				case strings.Contains(lower, "variable") || lower == "env":
					colVar = i
				case strings.Contains(lower, "default"):
					colDef = i
				case strings.Contains(lower, "description"):
					colDesc = i
				}
			}
			if colVar >= 0 {
				inTable = true
				headerSeen = false
			}
			continue
		}

		// ── Skip separator row (--- / :---: / ---:) ───────────────────────
		if isSeparator(cols) {
			headerSeen = true
			continue
		}

		if !headerSeen || colVar < 0 || colVar >= len(cols) {
			continue
		}

		// ── Extract variable name ─────────────────────────────────────────
		varCell := cols[colVar]
		varName := ""

		// Try backtick-quoted first (some READMEs use `VAR_NAME`)
		if m := backtickWord.FindStringSubmatch(varCell); m != nil && isEnvVarName(m[1]) {
			varName = m[1]
		} else {
			// Fall back to plain SCREAMING_SNAKE_CASE identifier in the cell
			candidate := strings.TrimSpace(varCell)
			if isEnvVarName(candidate) {
				varName = candidate
			}
		}
		if varName == "" {
			continue
		}

		ev := envVar{Name: varName}

		// ── Extract default value ─────────────────────────────────────────
		if colDef >= 0 && colDef < len(cols) {
			raw := strings.TrimSpace(cols[colDef])
			// Prefer backtick-quoted value; fall back to plain text
			if m := backtickWord.FindStringSubmatch(raw); m != nil {
				ev.Default = m[1]
			} else if raw != "" && raw != "-" && raw != "—" && raw != "N/A" {
				// Skip shell-variable interpolations like ${BASE}/foo — not a real default
				if !strings.Contains(raw, "${") {
					ev.Default = raw
				}
			}
		}

		// ── Extract description ───────────────────────────────────────────
		if colDesc >= 0 && colDesc < len(cols) {
			ev.Description = strings.TrimSpace(cols[colDesc])
		}

		// If no dedicated Default column, look for "Default: `x`" in description
		if ev.Default == "" && ev.Description != "" {
			if m := defaultInDesc.FindStringSubmatch(ev.Description); m != nil {
				ev.Default = m[1]
			}
		}

		vars = append(vars, ev)
	}

	// Deduplicate: keep first occurrence of each env var name.
	// (Some READMEs list inherited PingBase vars and then product-specific ones,
	// causing the same var to appear twice.)
	seen := map[string]bool{}
	out := vars[:0]
	for _, v := range vars {
		if !seen[v.Name] {
			seen[v.Name] = true
			out = append(out, v)
		}
	}
	return out
}

func splitRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func isSeparator(cols []string) bool {
	if len(cols) == 0 {
		return false
	}
	nonEmpty := 0
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		nonEmpty++
		for _, ch := range c {
			if ch != '-' && ch != ':' && ch != ' ' {
				return false
			}
		}
	}
	return nonEmpty > 0
}

// isEnvVarName returns true if s looks like a SCREAMING_SNAKE_CASE env var name
// (uppercase letters, digits, underscores; starts with a letter; at least 2 chars).
func isEnvVarName(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return false
	}
	if !unicode.IsUpper(rune(s[0])) {
		return false
	}
	for _, ch := range s {
		if !unicode.IsUpper(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return false
		}
	}
	return true
}

// ─────────────────────────── skip logic ──────────────────────────────────────

func shouldSkip(name string) bool {
	if skipExact[name] {
		return true
	}
	for _, pfx := range skipPrefixes {
		if strings.HasPrefix(name, pfx) {
			return true
		}
	}
	for _, suf := range skipSuffixes {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// ─────────────────────────── field derivation ────────────────────────────────

type field struct {
	name string
	typ  string
}

// deriveField converts an envVar into a Go field (name + type) and a JSON tag.
func deriveField(ev envVar, prefixes []string) (field, string) {
	name := ev.Name

	// Strip product-specific prefix when present
	for _, pfx := range prefixes {
		if strings.HasPrefix(name, pfx) {
			name = name[len(pfx):]
			break
		}
	}

	goName := segmentsToGoName(strings.Split(name, "_"))
	jsonTag := goNameToJSONTag(goName)

	return field{name: goName, typ: inferType(ev.Name, ev.Default)}, jsonTag
}

// segmentsToGoName converts a slice of UPPER_CASE segments to PascalCase,
// preserving known acronyms in full uppercase.
func segmentsToGoName(parts []string) string {
	var sb strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if acronyms[p] {
			sb.WriteString(p)
		} else {
			sb.WriteString(strings.ToUpper(p[:1]))
			sb.WriteString(strings.ToLower(p[1:]))
		}
	}
	return sb.String()
}

// goNameToJSONTag lowercases the leading "word" of a Go PascalCase identifier
// to produce a lowerCamelCase JSON tag.
//
//	AdminPort     → adminPort
//	LDAPPort      → ldapPort    (5 leading uppercase → acronym ends at i-1)
//	FIPSModeOn    → fipsModeOn
//	HTTPSPort     → httpsPort
func goNameToJSONTag(s string) string {
	if s == "" {
		return s
	}
	i := 0
	for i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		i++
	}
	switch {
	case i == 0:
		return s
	case i == 1:
		return strings.ToLower(s[:1]) + s[1:]
	case i == len(s):
		return strings.ToLower(s)
	default:
		// Multiple leading uppercase followed by lowercase:
		// field[:i-1] is the acronym; field[i-1:] starts the next TitleCase word.
		return strings.ToLower(s[:i-1]) + s[i-1:]
	}
}

// inferType returns the Go type string for an env var.
//
// Priority order:
//  1. Explicit bool default ("true" → *bool, "false" → bool)
//  2. Pure numeric default ("9031", "-1") → int32
//  3. Non-empty non-numeric default (e.g. "10000 KB", "STANDALONE") → string
//     (a known non-integer default overrides all name-based heuristics)
//  4. No default → name-based bool heuristics → name-based int heuristics → string
func inferType(envName, defaultVal string) string {
	def := strings.ToLower(strings.TrimSpace(defaultVal))

	// 1. Explicit boolean default
	switch def {
	case "true":
		// *bool: default=true means nil should keep chart behaviour; false must
		// be expressible without the zero-value ambiguity.
		return "*bool"
	case "false":
		return "bool"
	}

	// 2. Pure numeric default (including negative integers like -1)
	if isNumericLiteral(def) {
		return "int32"
	}

	// 3. Non-empty non-numeric default → definitely a string type
	if def != "" {
		return "string"
	}

	// 4. No default: fall back to name-based heuristics
	for _, pat := range boolContains {
		if strings.Contains(envName, pat) {
			return "bool"
		}
	}
	for _, suf := range intSuffixes {
		if strings.HasSuffix(envName, suf) {
			return "int32"
		}
	}
	return "string"
}

// isNumericLiteral returns true for integers (including negative) like "9031", "-1", "600".
func isNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for _, ch := range s {
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return true
}

// ─────────────────────────── comment builder ─────────────────────────────────

func buildComment(goField string, ev envVar) string {
	var sb strings.Builder
	sb.WriteString(goField)
	sb.WriteString(" maps to ")
	sb.WriteString(ev.Name)
	if ev.Default != "" {
		sb.WriteString(". Default: ")
		sb.WriteString(ev.Default)
	}
	if d := truncate(ev.Description, 90); d != "" {
		sb.WriteString(". ")
		sb.WriteString(d)
	}
	sb.WriteString(".")
	return sb.String()
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ─────────────────────────── existing-fields loader ──────────────────────────

// loadExistingJSONTags reads a Go source file and returns the set of json tag
// base names already declared (e.g. "enginePort", "ldapPort").
func loadExistingJSONTags(path string) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot open %s: %v\n", path, err)
		return map[string]bool{}
	}
	defer f.Close()

	existing := map[string]bool{}
	re := regexp.MustCompile(`json:"([^",]+)`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			existing[m[1]] = true
		}
	}
	return existing
}

// ─────────────────────────── HTTP fetch ──────────────────────────────────────

func fetchURL(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

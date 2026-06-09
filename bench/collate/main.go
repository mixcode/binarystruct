// Copyright 2026 github.com/mixcode

// Command collate parses two `go test -bench` outputs (one default/unsafe build,
// one built with -tags safe_binarystruct) from the bench/ suite and regenerates
// the comparison table inside the <!-- BENCH:START -->…<!-- BENCH:END --> region
// of README.md / README_ja.md. Invoked by `make bench`; not meant to be run by
// hand. Numbers are machine-dependent — the table is stamped accordingly.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type stat struct {
	ns     float64
	allocs float64
	n      int
}

func (s *stat) add(ns, allocs float64) { s.ns += ns; s.allocs += allocs; s.n++ }
func (s stat) ok() bool                { return s.n > 0 }
func (s stat) avgNs() float64          { return s.ns / float64(s.n) }
func (s stat) avgAllocs() float64      { return s.allocs / float64(s.n) }
func (s stat) cell() string {
	if !s.ok() {
		return "—"
	}
	return fmt.Sprintf("%s ns / %d allocs", commas(int64(s.avgNs()+0.5)), int64(s.avgAllocs()+0.5))
}

// key is "Workload_Op", e.g. "IntSlice_Marshal".
type modeStats struct{ safe, unsafe, codegen stat }

var benchLine = regexp.MustCompile(`^Benchmark(\S+?)(?:-\d+)?\s+\d+\s+([\d.]+)\s+ns/op(?:\s+([\d.]+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`)

// parse reads a `go test -bench` file; runtimeCol selects which column a
// Benchmark<Runtime_…> line feeds ("safe" or "unsafe"); Codegen_… always feeds codegen.
func parse(path, runtimeCol string, m map[string]*modeStats) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, ln := range strings.Split(string(data), "\n") {
		g := benchLine.FindStringSubmatch(ln)
		if g == nil {
			continue
		}
		name := g[1] // e.g. "Runtime_IntSlice_Marshal"
		ns, _ := strconv.ParseFloat(g[2], 64)
		allocs, _ := strconv.ParseFloat(g[4], 64)
		var col, key string
		switch {
		case strings.HasPrefix(name, "Runtime_"):
			col, key = runtimeCol, strings.TrimPrefix(name, "Runtime_")
		case strings.HasPrefix(name, "Codegen_"):
			col, key = "codegen", strings.TrimPrefix(name, "Codegen_")
		default:
			continue
		}
		ms, ok := m[key]
		if !ok {
			ms = &modeStats{}
			m[key] = ms
		}
		switch col {
		case "safe":
			ms.safe.add(ns, allocs)
		case "unsafe":
			ms.unsafe.add(ns, allocs)
		case "codegen":
			ms.codegen.add(ns, allocs)
		}
	}
	return nil
}

// workload/op ordering for stable, readable output.
var workloadOrder = []string{"Header", "IntSlice", "Record", "Nested"}
var opOrder = []string{"Marshal", "Unmarshal"}

func sortedKeys(m map[string]*modeStats) []string {
	rank := func(k string) (int, int, string) {
		w, op := k, ""
		if i := strings.LastIndex(k, "_"); i >= 0 {
			w, op = k[:i], k[i+1:]
		}
		wr, opr := len(workloadOrder), len(opOrder)
		for i, x := range workloadOrder {
			if x == w {
				wr = i
			}
		}
		for i, x := range opOrder {
			if x == op {
				opr = i
			}
		}
		return wr, opr, k
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a1, a2, a3 := rank(keys[i])
		b1, b2, b3 := rank(keys[j])
		if a1 != b1 {
			return a1 < b1
		}
		if a2 != b2 {
			return a2 < b2
		}
		return a3 < b3
	})
	return keys
}

func speedup(ms *modeStats) string {
	if !ms.unsafe.ok() || !ms.codegen.ok() || ms.codegen.avgNs() == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f×", ms.unsafe.avgNs()/ms.codegen.avgNs())
}

func renderTable(m map[string]*modeStats, header [6]string, stamp string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", header[0], header[1], header[2], header[3], header[4], header[5])
	b.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, k := range sortedKeys(m) {
		ms := m[k]
		w, op := k, ""
		if i := strings.LastIndex(k, "_"); i >= 0 {
			w, op = k[:i], k[i+1:]
		}
		fmt.Fprintf(&b, "| **%s** | %s | %s | %s | %s | %s |\n",
			w, op, ms.safe.cell(), ms.unsafe.cell(), ms.codegen.cell(), speedup(ms))
	}
	b.WriteString("\n")
	b.WriteString(stamp)
	return b.String()
}

var region = regexp.MustCompile(`(?s)(<!-- BENCH:START[^\n]*-->\n).*?(\n<!-- BENCH:END -->)`)

func writeRegion(path, body string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !region.Match(data) {
		return fmt.Errorf("%s: no <!-- BENCH:START -->…<!-- BENCH:END --> region found", path)
	}
	out := region.ReplaceAll(data, []byte("${1}"+strings.ReplaceAll(body, "$", "$$")+"${2}"))
	return os.WriteFile(path, out, 0o644)
}

func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	r := strings.Join(parts, ",")
	if neg {
		r = "-" + r
	}
	return r
}

func main() {
	unsafeFile := flag.String("unsafe", "", "go test -bench output from the default (unsafe) build")
	safeFile := flag.String("safe", "", "go test -bench output from the -tags safe_binarystruct build")
	readme := flag.String("readme", "", "README.md to update (English table)")
	readmeJa := flag.String("readme-ja", "", "README_ja.md to update (Japanese table)")
	flag.Parse()

	if *unsafeFile == "" || *safeFile == "" {
		fmt.Fprintln(os.Stderr, "collate: -unsafe and -safe are required")
		os.Exit(2)
	}
	m := map[string]*modeStats{}
	if err := parse(*unsafeFile, "unsafe", m); err != nil {
		fmt.Fprintln(os.Stderr, "collate:", err)
		os.Exit(1)
	}
	if err := parse(*safeFile, "safe", m); err != nil {
		fmt.Fprintln(os.Stderr, "collate:", err)
		os.Exit(1)
	}
	if len(m) == 0 {
		fmt.Fprintln(os.Stderr, "collate: no benchmark lines parsed — did the runs produce output?")
		os.Exit(1)
	}

	enStamp := fmt.Sprintf("> Measured with %s on this machine via `make bench` (mean of the run). Numbers are hardware-dependent — **re-run `make bench` for your environment.** Lower is better; speedup = unsafe ÷ codegen.", runtime.Version())
	jaStamp := fmt.Sprintf("> %s にてこのマシンで `make bench` により測定（実行の平均）。数値はハードウェア依存です。**お使いの環境では `make bench` を再実行してください。** 数値は小さいほど高速。高速化率 = unsafe ÷ codegen。", runtime.Version())

	enHeader := [6]string{"Workload", "Op", "Safe (runtime)", "Unsafe (runtime)", "Codegen", "Codegen speedup"}
	jaHeader := [6]string{"ワークロード", "処理", "Safe（ランタイム）", "Unsafe（ランタイム）", "コード生成", "コード生成の高速化"}

	if *readme != "" {
		if err := writeRegion(*readme, renderTable(m, enHeader, enStamp)); err != nil {
			fmt.Fprintln(os.Stderr, "collate:", err)
			os.Exit(1)
		}
		fmt.Printf("updated %s\n", *readme)
	}
	if *readmeJa != "" {
		if err := writeRegion(*readmeJa, renderTable(m, jaHeader, jaStamp)); err != nil {
			fmt.Fprintln(os.Stderr, "collate:", err)
			os.Exit(1)
		}
		fmt.Printf("updated %s\n", *readmeJa)
	}
	// Also echo the English table to stdout for a quick look.
	fmt.Println()
	fmt.Print(renderTable(m, enHeader, enStamp))
}

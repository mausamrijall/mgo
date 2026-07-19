package main

// Minimal terminal UI: color when stdout is a TTY, plain otherwise.
// Deliberately stdlib-only — the CLI's only dependency is cobra.

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var isTTY = func() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}()

func paint(code, s string) string {
	if !isTTY {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string  { return paint("1", s) }
func green(s string) string { return paint("32", s) }
func red(s string) string   { return paint("31", s) }
func cyan(s string) string  { return paint("36", s) }
func dim(s string) string   { return paint("2", s) }

func step(format string, args ...any) {
	fmt.Printf("%s %s\n", green("✓"), fmt.Sprintf(format, args...))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", red("✗"), fmt.Sprintf(format, args...))
}

// choose prompts a numbered single-choice question on stdin.
func choose(question string, options []string, def string) string {
	fmt.Printf("\n%s\n", bold(question))
	for i, opt := range options {
		marker := " "
		if opt == def {
			marker = dim(" (default)")
		} else {
			marker = ""
		}
		fmt.Printf("  %s %s%s\n", cyan(strconv.Itoa(i+1)+")"), opt, marker)
	}
	fmt.Printf("%s ", dim("choice ["+def+"]:"))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(options) {
		return options[n-1]
	}
	for _, opt := range options {
		if line == opt {
			return opt
		}
	}
	fmt.Println(dim("unrecognized — using " + def))
	return def
}

// ask prompts for a free-text answer.
func ask(question, def string) string {
	if def != "" {
		fmt.Printf("%s %s ", bold(question), dim("["+def+"]:"))
	} else {
		fmt.Printf("%s ", bold(question+":"))
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

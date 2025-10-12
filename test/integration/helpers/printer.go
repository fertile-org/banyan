package helpers

import (
	"fmt"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
)

// Printer provides formatted output for test scripts
type Printer struct {
	StepNumber int
}

// NewPrinter creates a new printer
func NewPrinter() *Printer {
	return &Printer{StepNumber: 0}
}

// Title prints a test title
func (p *Printer) Title(title string) {
	fmt.Printf("\n%s🧪 %s%s\n", ColorGreen, title, ColorReset)
	fmt.Println(repeat("=", 60))
}

// Step prints a test step
func (p *Printer) Step(description string) {
	p.StepNumber++
	fmt.Printf("\n%s===> Step %d: %s%s\n", ColorYellow, p.StepNumber, description, ColorReset)
}

// Success prints a success message
func (p *Printer) Success(message string) {
	fmt.Printf("%s✓ %s%s\n", ColorGreen, message, ColorReset)
}

// Error prints an error message
func (p *Printer) Error(message string) {
	fmt.Printf("%s✗ %s%s\n", ColorRed, message, ColorReset)
}

// Info prints an informational message
func (p *Printer) Info(message string) {
	fmt.Printf("%s%s%s\n", ColorCyan, message, ColorReset)
}

// Warning prints a warning message
func (p *Printer) Warning(message string) {
	fmt.Printf("%s⚠ %s%s\n", ColorYellow, message, ColorReset)
}

// Result prints the final test result
func (p *Printer) Result(passed bool, message string) {
	fmt.Println()
	if passed {
		fmt.Printf("%s✅ %s%s\n", ColorGreen, message, ColorReset)
	} else {
		fmt.Printf("%s❌ %s%s\n", ColorRed, message, ColorReset)
	}
}

// Code prints code or command output
func (p *Printer) Code(code string) {
	fmt.Printf("%s%s%s\n", ColorBlue, code, ColorReset)
}

// repeat repeats a string n times
func repeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

package ui

import (
	"fmt"
	"strings"
)

// --- ANSI Color Codes & UI Helpers ---
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
	ColorBold   = "\033[1m"

	IconCheck = "✓"
	IconCross = "✗"
	IconWarn  = "!"
	IconInfo  = "ℹ"
)

func PrintStyled(icon, color, label, msg string) {
	fmt.Printf("\r\033[2K%s%s %s%s%s %s\n", color, icon, ColorBold, label, ColorReset, msg)
}

func LogSuccess(msg string) { PrintStyled(IconCheck, ColorGreen, "[OK]  ", msg) }
func LogError(msg string)   { PrintStyled(IconCross, ColorRed, "[ERR] ", msg) }
func LogWarn(msg string)    { PrintStyled(IconWarn, ColorYellow, "[WARN]", msg) }
func LogInfo(msg string)    { PrintStyled(IconInfo, ColorBlue, "[INFO]", msg) }

func PrintHeader(msg string) {
	fmt.Printf("\n%s%s--- %s ---%s\n", ColorBold, ColorCyan, msg, ColorReset)
}

func PrintProgress(current, total int) {
	const width = 40
	if total <= 0 {
		return
	}
	percent := float64(current) / float64(total)
	if percent > 1.0 {
		percent = 1.0
	}
	filled := int(percent * float64(width))

	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">" + strings.Repeat(".", width-filled-1)
	} else {
		bar = strings.Repeat("=", width)
	}

	fmt.Printf("\r\033[2K%s[%s]%s %3.0f%% (%d/%d)", ColorCyan, bar, ColorReset, percent*100, current, total)
}

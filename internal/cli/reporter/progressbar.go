package reporter

import (
	"fmt"

	"github.com/pterm/pterm"
)

// getProgressBarString возвращает progress bar в виде строки
func getProgressBarString(percent int, width int) string {
	// Количество закрашенных блоков
	filled := (percent * width) / 100
	if filled > width {
		filled = width
	}

	bar := ""
	// Закрашенная часть
	bar += pterm.FgCyan.Sprint(repeat("█", filled))
	// Пустая часть
	bar += pterm.FgGray.Sprint(repeat("░", width-filled))

	return fmt.Sprintf("Testing: [%s]", bar)
}

func repeat(s string, count int) string {
	res := ""
	for i := 0; i < count; i++ {
		res += s
	}
	return res
}

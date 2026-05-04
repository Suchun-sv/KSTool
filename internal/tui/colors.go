package tui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/suchun/kstool/internal/model"
)

func statusColor(s model.Status) tcell.Color {
	switch s {
	case model.StatusRunning:
		return tcell.ColorGreen
	case model.StatusComplete:
		return tcell.ColorBlue
	case model.StatusFailed:
		return tcell.ColorRed
	case model.StatusPending:
		return tcell.ColorWhite
	}
	return tcell.ColorWhite
}

func gpuModelColor(g model.GPU) tcell.Color {
	if g.Idle {
		return tcell.ColorGray
	}
	switch g.Model {
	case "H200":
		return tcell.ColorGold
	case "H100":
		return tcell.ColorPurple
	case "A100":
		return tcell.ColorBlue
	}
	return tcell.ColorGray
}

func gpuCountColor(count int) tcell.Color {
	switch {
	case count <= 0:
		return tcell.ColorWhite
	case count == 1:
		return tcell.ColorYellow
	case count == 2:
		return tcell.ColorOrange
	default:
		return tcell.ColorRed
	}
}

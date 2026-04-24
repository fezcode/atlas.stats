package ui

import "github.com/charmbracelet/lipgloss"

// Phosphor-CRT telemetry palette.
// Amber primary on true-black, cyan numeric readouts, red criticals —
// evocative of 1970s engineering workstation displays.
var (
	ColBG       = lipgloss.Color("#000000")
	ColChrome   = lipgloss.Color("#3A3226") // structural borders
	ColDim      = lipgloss.Color("#7A6A4A") // secondary labels
	ColText     = lipgloss.Color("#D9C79C") // body text
	ColAmber    = lipgloss.Color("#FFB000") // primary phosphor
	ColAmberHot = lipgloss.Color("#FF7A00") // elevated
	ColRed      = lipgloss.Color("#FF3D4A") // critical
	ColCyan     = lipgloss.Color("#7DE3FF") // numeric readouts
	ColGreen    = lipgloss.Color("#84F5A3") // nominal
	ColPaper    = lipgloss.Color("#F5E6D3") // headings
)

// Reusable styles.
var (
	sBorder       = lipgloss.NewStyle().Foreground(ColChrome)
	sLabel        = lipgloss.NewStyle().Foreground(ColDim)
	sText         = lipgloss.NewStyle().Foreground(ColText)
	sValue        = lipgloss.NewStyle().Foreground(ColCyan).Bold(true)
	sPaper        = lipgloss.NewStyle().Foreground(ColPaper).Bold(true)
	sAmber        = lipgloss.NewStyle().Foreground(ColAmber).Bold(true)
	sHot          = lipgloss.NewStyle().Foreground(ColAmberHot).Bold(true)
	sCrit         = lipgloss.NewStyle().Foreground(ColRed).Bold(true)
	sGood         = lipgloss.NewStyle().Foreground(ColGreen).Bold(true)
	sDim          = lipgloss.NewStyle().Foreground(ColDim)
	sRec          = lipgloss.NewStyle().Foreground(ColRed).Bold(true)
	sSectionTitle = lipgloss.NewStyle().Foreground(ColPaper).Bold(true)
	sSectionKey   = lipgloss.NewStyle().Foreground(ColAmber).Bold(true)

	sFooterKey  = lipgloss.NewStyle().Foreground(ColAmber).Bold(true)
	sFooterText = lipgloss.NewStyle().Foreground(ColDim)

	sMastTitle = lipgloss.NewStyle().Foreground(ColAmber).Bold(true)
	sMastClock = lipgloss.NewStyle().Foreground(ColPaper).Bold(true)
)

// levelStyle returns the style matching the numeric severity of `pct` (0..100).
func levelStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 90:
		return sCrit
	case pct >= 75:
		return sHot
	case pct >= 50:
		return sAmber
	default:
		return sGood
	}
}

// statusPill returns a short bracketed severity pill.
func statusPill(pct float64) string {
	switch {
	case pct >= 90:
		return sCrit.Render("[ CRIT ]")
	case pct >= 75:
		return sHot.Render("[ ELEV ]")
	case pct >= 50:
		return sAmber.Render("[  OK  ]")
	default:
		return sGood.Render("[ NOM  ]")
	}
}

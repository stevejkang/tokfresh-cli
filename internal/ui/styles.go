package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		ExitAltScreen()
		os.Exit(1)
	}()
}

var (
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	MutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	BoldStyle    = lipgloss.NewStyle().Bold(true)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}).
			Bold(true)
	HeaderDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "238"})
)

func BrandHeader() string {
	return HeaderStyle.Render("⚡ TokFresh") + "\n" + HeaderDividerStyle.Render("─────────────────────────────────────────")
}

var inAltScreen bool

func EnterAltScreen() {
	fmt.Fprint(os.Stdout, "\033[?1049h\033[H\033[2J")
	inAltScreen = true
}

func ExitAltScreen() {
	if !inAltScreen {
		return
	}
	fmt.Fprint(os.Stdout, "\033[?1049l")
	inAltScreen = false
}

func ClearScreen() {
	fmt.Fprint(os.Stdout, "\033[H\033[2J")
}

func ClearAndBrand() {
	ClearScreen()
	fmt.Fprintln(os.Stdout, BrandHeader())
	fmt.Fprintln(os.Stdout)
}

func TokFreshTheme() *huh.Theme {
	t := huh.ThemeBase()

	var (
		normalFg = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
		blue     = lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}
		green    = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#4ADE80"}
		red      = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}
		gray     = lipgloss.AdaptiveColor{Light: "244", Dark: "249"}
		dimGray  = lipgloss.AdaptiveColor{Light: "250", Dark: "243"}
	)

	t.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")
	t.Focused.Base = t.Focused.Base.BorderForeground(lipgloss.AdaptiveColor{Light: "238", Dark: "238"})
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(normalFg).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(blue).Bold(true).MarginBottom(0)
	t.Focused.Description = t.Focused.Description.Foreground(gray)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(red)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(red)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(blue)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(blue)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(blue)
	t.Focused.Option = t.Focused.Option.Foreground(normalFg)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(blue)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(green)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(green).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(gray).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(normalFg)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(lipgloss.AdaptiveColor{Light: "#FFF", Dark: "#000"}).Background(blue)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(normalFg).Background(dimGray)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(blue)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "246"})
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(blue)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	t.Help = help.New().Styles
	t.Help.ShortKey = t.Help.ShortKey.Foreground(gray)
	t.Help.ShortDesc = t.Help.ShortDesc.Foreground(dimGray)
	t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(dimGray)

	return t
}

package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/stevejkang/tokfresh-cli/internal/config"
)

func RenderStatusTable(instances []config.Instance) string {
	rows := make([][]string, len(instances))
	for i, inst := range instances {
		created := inst.CreatedAt
		if len(created) >= 10 {
			created = created[:10]
		}
		rows[i] = []string{inst.Name, inst.Schedule, inst.Timezone, created}
	}

	t := table.New().
		Headers("Worker", "Start", "Timezone", "Created").
		Rows(rows...).
		Border(lipgloss.NormalBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

	return t.String()
}

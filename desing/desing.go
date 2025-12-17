package desing

// TODO: Código de la barra de progreso comentado para volver al comportamiento anterior
/*
import (
	"fmt"
	"strings"
	"time"

	"v2/internal/ram"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	padding  = 2
	maxWidth = 80
)

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render

// ProgressUpdateMsg es un mensaje para actualizar el progreso del benchmark
type ProgressUpdateMsg struct {
	Percent      float64
	Iteration    int
	Elapsed      time.Duration
	TargetTime   time.Duration
	CurrentSpeed string
}

// BenchmarkCompleteMsg indica que el benchmark ha terminado
type BenchmarkCompleteMsg struct{}

// model para la barra de progreso del benchmark de RAM
type model struct {
	progress     progress.Model
	percent      float64
	iteration    int
	elapsed      time.Duration
	targetTime   time.Duration
	currentSpeed string
	complete     bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.complete {
			return m, tea.Quit
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - padding*2 - 4
		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}
		return m, nil

	case ProgressUpdateMsg:
		m.percent = msg.Percent
		m.iteration = msg.Iteration
		m.elapsed = msg.Elapsed
		m.targetTime = msg.TargetTime
		m.currentSpeed = msg.CurrentSpeed
		cmd := m.progress.SetPercent(msg.Percent)
		return m, cmd

	case BenchmarkCompleteMsg:
		m.complete = true
		m.percent = 1.0
		cmd := m.progress.SetPercent(1.0)
		return m, cmd

	// FrameMsg is sent when the progress bar wants to animate itself
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	default:
		return m, nil
	}
}

func (m model) View() string {
	pad := strings.Repeat(" ", padding)

	var status string
	if m.complete {
		status = fmt.Sprintf(" Benchmark completado - %d iteraciones en %v",
			m.iteration, m.elapsed)
	} else {
		elapsedStr := formatDuration(m.elapsed)
		targetStr := formatDuration(m.targetTime)
		status = fmt.Sprintf("Iteración %d | Tiempo: %s / %s",
			m.iteration, elapsedStr, targetStr)
		if m.currentSpeed != "" {
			status += fmt.Sprintf(" | %s", m.currentSpeed)
		}
	}

	return "\n" +
		pad + "Benchmark de RAM en progreso...\n\n" +
		pad + m.progress.View() + "\n\n" +
		pad + helpStyle(status) + "\n" +
		pad + helpStyle("Presiona cualquier tecla para continuar cuando termine")
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// NewRAMProgressModel crea un nuevo modelo para el benchmark de RAM
func NewRAMProgressModel() model {
	return model{
		progress: progress.New(progress.WithDefaultGradient()),
		percent:  0.0,
	}
}

// benchmarkRunner ejecuta el benchmark y envía actualizaciones de progreso
type benchmarkRunner struct {
	config       ram.RAMBenchmarkConfig
	progressChan chan ProgressUpdateMsg
	resultChan   chan benchmarkResult
	startTime    time.Time
}

func newBenchmarkRunner(config ram.RAMBenchmarkConfig) *benchmarkRunner {
	return &benchmarkRunner{
		config:       config,
		progressChan: make(chan ProgressUpdateMsg, 10),
		resultChan:   make(chan benchmarkResult, 1),
	}
}

func (br *benchmarkRunner) run() {
	defer close(br.progressChan)
	defer close(br.resultChan)

	br.startTime = time.Now()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Ejecutar el benchmark en una goroutine
	benchmarkDone := make(chan benchmarkResult, 1)
	go func() {
		result, err := ram.RunRAMBenchmark(br.config)
		benchmarkDone <- benchmarkResult{result: result, err: err}
	}()

	// Monitorear el progreso
	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(br.startTime)
			percent := float64(elapsed) / float64(br.config.TargetDuration)
			if percent > 0.99 {
				percent = 0.99 // No llegar a 1.0 hasta que termine
			}

			// Estimar iteración
			estimatedIteration := int(float64(br.config.MinIterations) * percent)
			if estimatedIteration < 1 {
				estimatedIteration = 1
			}

			select {
			case br.progressChan <- ProgressUpdateMsg{
				Percent:    percent,
				Iteration:  estimatedIteration,
				Elapsed:    elapsed,
				TargetTime: br.config.TargetDuration,
			}:
			default:
			}

		case result := <-benchmarkDone:
			elapsed := time.Since(br.startTime)
			if result.result != nil {
				select {
				case br.progressChan <- ProgressUpdateMsg{
					Percent:    1.0,
					Iteration:  result.result.TotalIterations,
					Elapsed:    elapsed,
					TargetTime: br.config.TargetDuration,
				}:
				default:
				}
			}
			br.resultChan <- result
			return
		}
	}
}

type benchmarkResult struct {
	result *ram.RAMBenchmarkResult
	err    error
}

// RunRAMBenchmarkWithProgress ejecuta el benchmark de RAM con una barra de progreso
func RunRAMBenchmarkWithProgress(config ram.RAMBenchmarkConfig) (*ram.RAMBenchmarkResult, error) {
	m := NewRAMProgressModel()
	m.targetTime = config.TargetDuration

	runner := newBenchmarkRunner(config)
	go runner.run()

	// Crear el programa de tea
	program := tea.NewProgram(&benchmarkProgressModel{
		model:  m,
		runner: runner,
		config: config,
	})

	// Ejecutar el programa
	finalModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("error ejecutando UI: %w", err)
	}

	// Obtener el resultado del runner
	bmModel := finalModel.(*benchmarkProgressModel)
	return bmModel.result, bmModel.err
}

// benchmarkProgressModel integra el modelo con el runner del benchmark
type benchmarkProgressModel struct {
	model  model
	runner *benchmarkRunner
	config ram.RAMBenchmarkConfig
	result *ram.RAMBenchmarkResult
	err    error
}

func (b *benchmarkProgressModel) Init() tea.Cmd {
	return tea.Batch(
		b.checkProgress(),
		tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return tickMsg{}
		}),
	)
}

type tickMsg struct{}

func (b *benchmarkProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if b.model.complete {
			return b, tea.Quit
		}
		return b, nil

	case tea.WindowSizeMsg:
		newModel, cmd := b.model.Update(msg)
		b.model = newModel.(model)
		return b, cmd

	case tickMsg:
		return b, tea.Batch(
			b.checkProgress(),
			tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
				return tickMsg{}
			}),
		)

	case ProgressUpdateMsg:
		newModel, cmd := b.model.Update(msg)
		b.model = newModel.(model)
		return b, cmd

	case BenchmarkCompleteMsg:
		newModel, cmd := b.model.Update(msg)
		b.model = newModel.(model)
		return b, cmd

	case progress.FrameMsg:
		newModel, cmd := b.model.Update(msg)
		b.model = newModel.(model)
		return b, cmd

	default:
		return b, nil
	}
}

func (b *benchmarkProgressModel) checkProgress() tea.Cmd {
	return func() tea.Msg {
		// Verificar si hay resultado primero
		select {
		case result := <-b.runner.resultChan:
			b.result = result.result
			b.err = result.err
			if result.result != nil {
				return ProgressUpdateMsg{
					Percent:    1.0,
					Iteration:  result.result.TotalIterations,
					Elapsed:    result.result.TotalDuration,
					TargetTime: b.config.TargetDuration,
				}
			}
			return BenchmarkCompleteMsg{}
		default:
		}

		// Verificar actualizaciones de progreso
		select {
		case update, ok := <-b.runner.progressChan:
			if !ok {
				return BenchmarkCompleteMsg{}
			}
			return update
		default:
			return nil
		}
	}
}

func (b *benchmarkProgressModel) View() string {
	return b.model.View()
}
*/

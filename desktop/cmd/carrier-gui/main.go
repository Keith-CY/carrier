// Carrier Desktop GUI — a Gio-based agent installer/manager.
package main

import (
	"image/color"
	"log"
	"os"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"carrier/desktop/internal/client"
)

// Screens
const (
	screenConnect = iota
	screenAgents
	screenConfig
	screenInstall
	screenManage
)

type agentEntry struct {
	agent    client.Agent
	selected bool
	check    widget.Bool
	envVals  map[string]string // env key -> value
	envEdits map[string]*widget.Editor
	// management buttons
	btnStart  widget.Clickable
	btnStop   widget.Clickable
	btnStatus widget.Clickable
	btnLogs   widget.Clickable
	status    string
	logs      string
}

type appState struct {
	mu     sync.Mutex
	screen int

	// connection
	urlEditor   widget.Editor
	tokenEditor widget.Editor
	btnConnect  widget.Clickable
	connErr     string
	cli         *client.Client

	// agents
	agents     []*agentEntry
	btnNext    widget.Clickable
	btnBack    widget.Clickable
	btnInstall widget.Clickable
	listAgents widget.List
	listManage widget.List
	fetchErr   string

	// install status
	installMsg string
}

func main() {
	go func() {
		var w app.Window
		w.Option(app.Title("Carrier Desktop"), app.Size(unit.Dp(720), unit.Dp(560)))
		if err := run(&w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error {
	th := material.NewTheme()
	state := &appState{}
	state.urlEditor.SetText("http://localhost:9090")
	state.urlEditor.SingleLine = true
	state.tokenEditor.SingleLine = true
	state.listAgents.Axis = layout.Vertical
	state.listManage.Axis = layout.Vertical

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			state.update(gtx, w)
			state.layout(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}

// update handles button clicks and async actions.
func (s *appState) update(gtx layout.Context, w *app.Window) {
	switch s.screen {
	case screenConnect:
		if s.btnConnect.Clicked(gtx) {
			url := strings.TrimSpace(s.urlEditor.Text())
			token := strings.TrimSpace(s.tokenEditor.Text())
			s.cli = client.New(url, token)
			s.connErr = ""
			go func() {
				err := s.cli.Healthz()
				s.mu.Lock()
				if err != nil {
					s.connErr = err.Error()
				} else {
					agents, ferr := s.cli.ListAgents()
					if ferr != nil {
						s.connErr = ferr.Error()
					} else {
						s.agents = make([]*agentEntry, len(agents))
						for i, a := range agents {
							s.agents[i] = &agentEntry{
								agent:    a,
								envVals:  map[string]string{},
								envEdits: map[string]*widget.Editor{},
							}
						}
						s.screen = screenAgents
					}
				}
				s.mu.Unlock()
				w.Invalidate()
			}()
		}
	case screenAgents:
		// sync checkbox state
		for _, ae := range s.agents {
			ae.selected = ae.check.Value
		}
		if s.btnNext.Clicked(gtx) {
			s.screen = screenConfig
		}
		if s.btnBack.Clicked(gtx) {
			s.screen = screenConnect
		}
	case screenConfig:
		if s.btnBack.Clicked(gtx) {
			s.screen = screenAgents
		}
		if s.btnInstall.Clicked(gtx) {
			s.installMsg = "Installing..."
			s.screen = screenInstall
			go func() {
				for _, ae := range s.agents {
					if !ae.selected {
						continue
					}
					// sync env values
					for k, ed := range ae.envEdits {
						ae.envVals[k] = ed.Text()
					}
					if err := s.cli.Install(ae.agent.ID); err != nil {
						s.mu.Lock()
						s.installMsg = "Install error: " + err.Error()
						s.mu.Unlock()
						w.Invalidate()
						return
					}
					if err := s.cli.Start(ae.agent.ID); err != nil {
						s.mu.Lock()
						s.installMsg = "Start error: " + err.Error()
						s.mu.Unlock()
						w.Invalidate()
						return
					}
				}
				// fetch statuses
				for _, ae := range s.agents {
					if !ae.selected {
						continue
					}
					st, err := s.cli.GetStatus(ae.agent.ID)
					if err != nil {
						ae.status = "unknown"
					} else {
						ae.status = st.Status
					}
				}
				s.mu.Lock()
				s.installMsg = "All agents installed and started."
				s.screen = screenManage
				s.mu.Unlock()
				w.Invalidate()
			}()
		}
	case screenInstall:
		// waiting for install goroutine
	case screenManage:
		for _, ae := range s.agents {
			if !ae.selected {
				continue
			}
			if ae.btnStart.Clicked(gtx) {
				id := ae.agent.ID
				entry := ae
				go func() {
					_ = s.cli.Start(id)
					if st, err := s.cli.GetStatus(id); err == nil {
						entry.status = st.Status
					}
					w.Invalidate()
				}()
			}
			if ae.btnStop.Clicked(gtx) {
				id := ae.agent.ID
				entry := ae
				go func() {
					_ = s.cli.Stop(id)
					if st, err := s.cli.GetStatus(id); err == nil {
						entry.status = st.Status
					}
					w.Invalidate()
				}()
			}
			if ae.btnStatus.Clicked(gtx) {
				id := ae.agent.ID
				entry := ae
				go func() {
					if st, err := s.cli.GetStatus(id); err == nil {
						entry.status = st.Status
					}
					w.Invalidate()
				}()
			}
			if ae.btnLogs.Clicked(gtx) {
				id := ae.agent.ID
				entry := ae
				go func() {
					if l, err := s.cli.Logs(id, 50); err == nil {
						entry.logs = l
					}
					w.Invalidate()
				}()
			}
		}
	}
}

// layout draws the current screen.
func (s *appState) layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.mu.Lock()
	screen := s.screen
	s.mu.Unlock()

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		switch screen {
		case screenConnect:
			return s.layoutConnect(gtx, th)
		case screenAgents:
			return s.layoutAgents(gtx, th)
		case screenConfig:
			return s.layoutConfig(gtx, th)
		case screenInstall:
			return s.layoutInstall(gtx, th)
		case screenManage:
			return s.layoutManage(gtx, th)
		}
		return layout.Dimensions{}
	})
}

func (s *appState) layoutConnect(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H5(th, "Connect to Carrier Daemon")
			lbl.Alignment = text.Middle
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Body1(th, "Daemon URL").Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(th, &s.urlEditor, "http://localhost:9090")
			return ed.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Body1(th, "API Token (optional)").Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(th, &s.tokenEditor, "token")
			return ed.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, &s.btnConnect, "Connect")
			return btn.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.connErr == "" {
				return layout.Dimensions{}
			}
			lbl := material.Body2(th, "Error: "+s.connErr)
			lbl.Color = color.NRGBA{R: 200, A: 255}
			return lbl.Layout(gtx)
		}),
	)
}

func (s *appState) layoutAgents(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.H5(th, "Select Agents to Install").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, &s.listAgents).Layout(gtx, len(s.agents), func(gtx layout.Context, i int) layout.Dimensions {
				ae := s.agents[i]
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.CheckBox(th, &ae.check, "").Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						desc := ae.agent.Name
						if ae.agent.Description != "" {
							desc += " — " + ae.agent.Description
						}
						return material.Body1(th, desc).Layout(gtx)
					}),
				)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Button(th, &s.btnBack, "Back").Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Button(th, &s.btnNext, "Next").Layout(gtx)
				}),
			)
		}),
	)
}

func (s *appState) layoutConfig(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Simple config: show a placeholder for env vars per selected agent
	var selected []*agentEntry
	for _, ae := range s.agents {
		if ae.selected {
			selected = append(selected, ae)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.H5(th, "Configure Agents").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(selected) == 0 {
				return material.Body1(th, "No agents selected. Go back and select agents.").Layout(gtx)
			}
			return material.List(th, &s.listAgents).Layout(gtx, len(selected), func(gtx layout.Context, i int) layout.Dimensions {
				ae := selected[i]
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.H6(th, ae.agent.Name).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Body2(th, "Environment variables can be configured after install via daemon API.").Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Button(th, &s.btnBack, "Back").Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Button(th, &s.btnInstall, "Install").Layout(gtx)
				}),
			)
		}),
	)
}

func (s *appState) layoutInstall(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.mu.Lock()
	msg := s.installMsg
	s.mu.Unlock()
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.H5(th, "Installing...").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Body1(th, msg).Layout(gtx)
		}),
	)
}

func (s *appState) layoutManage(gtx layout.Context, th *material.Theme) layout.Dimensions {
	var selected []*agentEntry
	for _, ae := range s.agents {
		if ae.selected {
			selected = append(selected, ae)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.H5(th, "Manage Agents").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, &s.listManage).Layout(gtx, len(selected), func(gtx layout.Context, i int) layout.Dimensions {
				ae := selected[i]
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						statusText := ae.agent.Name + "  [" + ae.status + "]"
						return material.H6(th, statusText).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(th, &ae.btnStart, "Start").Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(th, &ae.btnStop, "Stop").Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(th, &ae.btnStatus, "Refresh").Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(th, &ae.btnLogs, "Logs").Layout(gtx)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ae.logs == "" {
							return layout.Dimensions{}
						}
						lbl := material.Body2(th, ae.logs)
						lbl.MaxLines = 20
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				)
			})
		}),
	)
}

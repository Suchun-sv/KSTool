package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"

	"github.com/suchun/kstool/internal/config"
	"github.com/suchun/kstool/internal/editor"
	klog "github.com/suchun/kstool/internal/log"
	"github.com/suchun/kstool/internal/template"
)

const (
	createPage = "create-form"
	listPage   = "create-list"
)

// showCreateForm enters the env-config / create-job flow. onClose is invoked
// after a successful Apply or when the user exits the flow.
func showCreateForm(a *App, onClose func()) {
	f := &createFlow{app: a, onClose: onClose}
	f.showList()
}

type createFlow struct {
	app     *App
	onClose func()
	pop     func()
}

// remove removes any currently-displayed flow page.
func (f *createFlow) remove() {
	if f.pop != nil {
		f.pop()
		f.pop = nil
	}
}

func (f *createFlow) finish() {
	f.remove()
	if f.onClose != nil {
		f.onClose()
	}
}

// showList renders the list of saved env configs plus "Create new" / "Exit".
func (f *createFlow) showList() {
	f.remove()
	names, err := config.ListEnvConfigs(f.app.paths)
	if err != nil {
		f.app.showError(err)
		return
	}

	list := tview.NewList()
	list.SetBorder(true).SetTitle("Available Configurations").SetTitleAlign(tview.AlignLeft)
	list.AddItem("Create new configuration", "Build a fresh job from the base template", 'n', func() {
		f.openForm(nil, "")
	})
	for _, name := range names {
		nm := name
		list.AddItem(nm, "(l) load · (d) delete", 'l', func() { f.handlePickSaved(nm) })
	}
	list.AddItem("Back", "Return to job list", 'q', f.finish)

	list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyRune && ev.Rune() == 'd' {
			idx := list.GetCurrentItem()
			if idx > 0 && idx <= len(names) {
				name := names[idx-1]
				f.confirmDelete(name)
			}
			return nil
		}
		if ev.Key() == tcell.KeyEscape {
			f.finish()
			return nil
		}
		return ev
	})

	f.pop = f.app.pushPage(listPage, list)
}

func (f *createFlow) confirmDelete(name string) {
	f.app.showModal(fmt.Sprintf("Delete saved config %q?", name), []string{"Cancel", "Delete"}, func(label string) {
		if label != "Delete" {
			return
		}
		if err := config.DeleteEnvConfig(f.app.paths, name); err != nil {
			f.app.showError(err)
			return
		}
		f.showList()
	})
}

func (f *createFlow) handlePickSaved(name string) {
	cfg, err := config.LoadEnvConfig(f.app.paths, name)
	if err != nil {
		f.app.showError(err)
		return
	}
	f.app.showModal(fmt.Sprintf("Configuration %q loaded.\nWhat next?", name),
		[]string{"Apply", "Edit", "Back"}, func(label string) {
			switch label {
			case "Apply":
				go f.applyEnv(cfg)
			case "Edit":
				f.openForm(cfg, name)
			}
		})
}

// openForm builds the env-var form. When base is nil, a fresh config is
// derived from the on-disk template's defaults.
func (f *createFlow) openForm(base *config.EnvConfig, savedName string) {
	templateBytes, err := config.ReadTemplate(f.app.paths)
	if err != nil {
		f.app.showError(fmt.Errorf("read template: %w", err))
		return
	}
	vars := template.Extract(string(templateBytes))
	if len(vars) == 0 {
		f.app.showError(fmt.Errorf("template has no ${VAR:-default} placeholders"))
		return
	}

	cfg := &config.EnvConfig{}
	user := klog.CurrentUser()
	for _, v := range vars {
		val := v.Default
		if v.Name == "USER" {
			val = user
		}
		// Match the legacy "default-user" sentinel anywhere in the default
		// value, so things like "default-user-ws4" become "<user>-ws4".
		if strings.Contains(val, "default-user") {
			val = strings.ReplaceAll(val, "default-user", user)
		}
		cfg.Set(v.Name, val)
	}
	if base != nil {
		for _, e := range base.EnvVars {
			cfg.Set(e.Key, e.Value)
		}
	}
	cfg.Sort()

	f.remove()
	f.renderForm(cfg, templateBytes, savedName, false)
}

// renderForm rebuilds the form primitives. Called fresh, after a vim edit, or
// after toggling. modified indicates whether the form has unsaved input.
func (f *createFlow) renderForm(cfg *config.EnvConfig, templateBytes []byte, savedName string, modified bool) {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle("Job Configuration").SetTitleAlign(tview.AlignLeft)

	mod := modified
	addField := func(key string) {
		val, _ := cfg.Get(key)
		switch key {
		case "GPU_PRODUCT":
			opts := f.app.cfg.GPUProducts
			idx := indexOf(opts, val)
			form.AddDropDown(key, opts, idx, func(option string, _ int) {
				cfg.Set(key, option)
				mod = true
			})
		case "PRIORITY_CLASS":
			opts := f.app.cfg.PriorityClass
			idx := indexOf(opts, val)
			form.AddDropDown(key, opts, idx, func(option string, _ int) {
				cfg.Set(key, option)
				mod = true
			})
		default:
			form.AddInputField(key, val, 30, nil, func(text string) {
				cfg.Set(key, text)
				mod = true
			})
		}
	}

	keys := make([]string, 0, len(cfg.EnvVars))
	for _, e := range cfg.EnvVars {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	for _, k := range keys {
		addField(k)
	}

	back := func() {
		if mod {
			f.app.showModal("Discard unsaved changes?", []string{"Cancel", "Discard"}, func(label string) {
				if label == "Discard" {
					f.showList()
				}
			})
			return
		}
		f.showList()
	}

	form.AddButton("Edit in Vim (e)", func() { f.editInVim(cfg, templateBytes, savedName) })
	form.AddButton("Save (Ctrl+S)", func() { f.saveDialog(cfg, savedName) })
	form.AddButton("Apply (F5)", func() { go f.applyEnv(cfg) })
	form.AddButton("Back (Esc)", back)

	form.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape {
			back()
			return nil
		}
		if ev.Key() == tcell.KeyRune && ev.Rune() == 'e' {
			// Only treat 'e' as the vim shortcut when focus is on a button,
			// otherwise it would shadow the user typing into an input field.
			if _, isButton := f.app.app.GetFocus().(*tview.Button); isButton {
				f.editInVim(cfg, templateBytes, savedName)
				return nil
			}
		}
		return ev
	})

	help := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("Tab/Shift-Tab move · Enter selects · e edit-in-vim · Esc back")

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(form, 0, 1, true)
	flex.AddItem(help, 1, 0, false)

	f.remove()
	f.pop = f.app.pushPage(createPage, flex)
}

func (f *createFlow) editInVim(cfg *config.EnvConfig, templateBytes []byte, savedName string) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		f.app.showError(err)
		return
	}
	var edited []byte
	f.app.app.Suspend(func() {
		out, err := editor.Edit(".yaml", data, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "editor error: %v\n", err)
			return
		}
		edited = out
	})
	if edited == nil {
		return
	}
	var fresh config.EnvConfig
	if err := yaml.Unmarshal(edited, &fresh); err != nil {
		f.app.showError(fmt.Errorf("invalid YAML from editor: %w", err))
		return
	}
	cfg.EnvVars = fresh.EnvVars
	cfg.Sort()
	f.renderForm(cfg, templateBytes, savedName, true)
}

func (f *createFlow) saveDialog(cfg *config.EnvConfig, defaultName string) {
	input := tview.NewInputField().SetLabel("Config name: ").SetFieldWidth(24).SetText(defaultName)
	form := tview.NewForm().AddFormItem(input)
	form.SetBorder(true).SetTitle("Save Configuration").SetTitleAlign(tview.AlignLeft)

	var pop func()
	doSave := func() {
		name := input.GetText()
		if name == "" {
			f.app.showError(fmt.Errorf("config name cannot be empty"))
			return
		}
		if err := config.SaveEnvConfig(f.app.paths, name, cfg); err != nil {
			f.app.showError(err)
			return
		}
		pop()
		f.app.showMessage(fmt.Sprintf("Saved %q.", name))
	}
	form.AddButton("Save", doSave)
	form.AddButton("Cancel", func() { pop() })

	form.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape {
			pop()
			return nil
		}
		return ev
	})

	pop = f.app.pushModal(form)
}

// applyEnv renders the template, runs a server-side dry-run, then a real
// create. It runs on a goroutine; UI updates are queued back.
func (f *createFlow) applyEnv(cfg *config.EnvConfig) {
	templateBytes, err := config.ReadTemplate(f.app.paths)
	if err != nil {
		f.app.app.QueueUpdateDraw(func() { f.app.showError(err) })
		return
	}
	rendered := template.Render(string(templateBytes), cfg.AsMap())

	ctx, cancel := context.WithTimeout(f.app.ctx, 30*time.Second)
	defer cancel()
	if _, err := f.app.client.CreateJob(ctx, []byte(rendered), true); err != nil {
		f.app.app.QueueUpdateDraw(func() {
			f.app.showError(fmt.Errorf("dry-run failed: %w", err))
		})
		return
	}
	created, err := f.app.client.CreateJob(ctx, []byte(rendered), false)
	if err != nil {
		f.app.app.QueueUpdateDraw(func() { f.app.showError(err) })
		return
	}
	klog.Action("create", created.Name)
	f.app.app.QueueUpdateDraw(func() {
		f.app.showMessage(fmt.Sprintf("Job %s created.", created.Name))
		f.finish()
	})
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return 0
}

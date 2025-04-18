package src

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
)

const (
	configDir     = ".kstool"
	configListDir = "env_config_list"
	baseConfigURL = "https://raw.githubusercontent.com/Suchun-sv/KSTool/main/config/base_apply.yaml"
)

// Config represents the configuration for a job
type Config struct {
	User         string `yaml:"user"`
	QueueName    string `yaml:"queue_name"`
	ImageName    string `yaml:"image_name"`
	Command      string `yaml:"command"`
	CPUNum       string `yaml:"cpu_num"`
	MemoryNum    string `yaml:"memory_num"`
	GPUNum       string `yaml:"gpu_num"`
	GPUProduct   string `yaml:"gpu_product"`
	Mount        string `yaml:"mount"`
	WorkspacePVC string `yaml:"workspace_pvc"`
	NFSPath      string `yaml:"nfs_path"`
	NFSServer    string `yaml:"nfs_server"`
}

// CreateJobForm represents the form for creating a new job
type CreateJobForm struct {
	app          *tview.Application
	form         *tview.Form
	config       *Config
	onClose      func()
	configList   *tview.List
	flex         *tview.Flex
	currentPanel tview.Primitive
}

// initializeDirectories ensures all required directories exist
func initializeDirectories() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create .kstool directory
	kstoolDir := filepath.Join(homeDir, configDir)
	if err := os.MkdirAll(kstoolDir, 0755); err != nil {
		return fmt.Errorf("failed to create .kstool directory: %w", err)
	}

	// Create env_config_list directory
	configListPath := filepath.Join(kstoolDir, configListDir)
	if err := os.MkdirAll(configListPath, 0755); err != nil {
		return fmt.Errorf("failed to create env_config_list directory: %w", err)
	}

	return nil
}

// downloadBaseConfig downloads the base configuration file if it doesn't exist
func downloadBaseConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	baseConfigPath := filepath.Join(homeDir, configDir, "base_apply.yaml")
	if _, err := os.Stat(baseConfigPath); err == nil {
		return nil // File exists
	}

	// Download the file
	resp, err := http.Get(baseConfigURL)
	if err != nil {
		return fmt.Errorf("failed to download base config: %w", err)
	}
	defer resp.Body.Close()

	file, err := os.Create(baseConfigPath)
	if err != nil {
		return fmt.Errorf("failed to create base config file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write base config file: %w", err)
	}

	return nil
}

// loadConfigList loads all configuration files from the env_config_list directory
func loadConfigList() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configListPath := filepath.Join(homeDir, configDir, configListDir)
	files, err := os.ReadDir(configListPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read config list directory: %w", err)
	}

	var configs []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") && file.Name() != "base_apply.yaml" {
			configs = append(configs, strings.TrimSuffix(file.Name(), ".yaml"))
		}
	}
	return configs, nil
}

// loadConfig loads a specific configuration file
func loadConfig(name string) (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, configDir, configListDir, name+".yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// createConfigForm creates a form for editing configuration
func (f *CreateJobForm) createConfigForm(config *Config) *tview.Form {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle("Create/Edit Job Configuration").SetTitleAlign(tview.AlignLeft)

	// Track if the form has been modified
	modified := false

	// Add buttons with keyboard shortcuts at the top
	form.AddButton("Save Config (Ctrl+S)", func() {
		f.showSaveConfigDialog(config)
		modified = false
	})
	form.AddButton("Apply (F5)", func() {
		if err := applyJobConfig(*config); err != nil {
			showError(f.app, form, fmt.Sprintf("Failed to apply job: %v", err))
		} else {
			showMessage(f.app, form, "Job created successfully")
			modified = false
			f.onClose()
		}
	})
	form.AddButton("Back (Esc)", func() {
		if modified {
			modal := tview.NewModal().
				SetText("You have unsaved changes. Are you sure you want to go back?").
				AddButtons([]string{"Cancel", "Yes"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Yes" {
						f.showConfigList()
					} else {
						f.app.SetRoot(form, true)
					}
				})
			f.app.SetRoot(modal, true)
		} else {
			f.showConfigList()
		}
	})

	// Add separator
	form.AddTextView("", "", 0, 1, false, false)

	// Add GPU related fields first
	gpuOptions := []string{"NVIDIA-H200", "NVIDIA-H100-80GB-HBM3", "NVIDIA-A100-80GB-HBM3"}
	defaultIndex := 0
	for i, option := range gpuOptions {
		if option == config.GPUProduct {
			defaultIndex = i
			break
		}
	}
	form.AddDropDown("GPU Product", gpuOptions, defaultIndex, func(option string, index int) {
		config.GPUProduct = option
		modified = true
	})
	form.AddInputField("GPU Number", config.GPUNum, 30, nil, func(text string) {
		config.GPUNum = text
		modified = true
	})

	// Add other form fields
	form.AddInputField("Queue Name", config.QueueName, 30, nil, func(text string) {
		config.QueueName = text
		modified = true
	})
	form.AddInputField("Image Name", config.ImageName, 30, nil, func(text string) {
		config.ImageName = text
		modified = true
	})
	form.AddInputField("Command", config.Command, 30, nil, func(text string) {
		config.Command = text
		modified = true
	})
	form.AddInputField("CPU Number", config.CPUNum, 30, nil, func(text string) {
		config.CPUNum = text
		modified = true
	})
	form.AddInputField("Memory", config.MemoryNum, 30, nil, func(text string) {
		config.MemoryNum = text
		modified = true
	})
	form.AddInputField("Mount Path", config.Mount, 30, nil, func(text string) {
		config.Mount = text
		modified = true
	})
	form.AddInputField("Workspace PVC", config.WorkspacePVC, 30, nil, func(text string) {
		config.WorkspacePVC = text
		modified = true
	})
	form.AddInputField("NFS Path", config.NFSPath, 30, nil, func(text string) {
		config.NFSPath = text
		modified = true
	})
	form.AddInputField("NFS Server", config.NFSServer, 30, nil, func(text string) {
		config.NFSServer = text
		modified = true
	})

	// Set keyboard shortcuts
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyRune && event.Rune() == 's' && (event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0):
			f.showSaveConfigDialog(config)
			modified = false
			return nil
		case event.Key() == tcell.KeyF5:
			if err := applyJobConfig(*config); err != nil {
				showError(f.app, form, fmt.Sprintf("Failed to apply job: %v", err))
			} else {
				showMessage(f.app, form, "Job created successfully")
				modified = false
				f.onClose()
			}
			return nil
		case event.Key() == tcell.KeyEscape:
			if modified {
				modal := tview.NewModal().
					SetText("You have unsaved changes. Are you sure you want to go back?").
					AddButtons([]string{"Cancel", "Yes"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						if buttonLabel == "Yes" {
							f.showConfigList()
						} else {
							f.app.SetRoot(form, true)
						}
					})
				f.app.SetRoot(modal, true)
			} else {
				f.showConfigList()
			}
			return nil
		}
		return event
	})

	return form
}

// showSaveConfigDialog shows a dialog for saving the configuration
func (f *CreateJobForm) showSaveConfigDialog(config *Config) {
	// Create a flex container to hold both the text and input field
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Add the text
	text := tview.NewTextView().
		SetText("Enter configuration name:").
		SetTextAlign(tview.AlignCenter)
	flex.AddItem(text, 1, 0, false)

	// Add the input field
	inputField := tview.NewInputField().
		SetLabel("Config Name: ").
		SetFieldWidth(20)

	inputField.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			name := inputField.GetText()
			if name == "" {
				showError(f.app, f.currentPanel, "Configuration name cannot be empty")
				return
			}
			if err := f.saveConfig(name, config); err != nil {
				showError(f.app, f.currentPanel, fmt.Sprintf("Failed to save config: %v", err))
			} else {
				showMessage(f.app, f.currentPanel, "Configuration saved successfully")
				f.showConfigList() // Refresh the list
			}
		case tcell.KeyEscape:
			f.app.SetRoot(f.currentPanel, true)
		}
	})

	flex.AddItem(inputField, 1, 0, true)

	// Create the modal with buttons
	modal := tview.NewModal().
		SetText("").
		AddButtons([]string{"Cancel", "Save"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Save" {
				name := inputField.GetText()
				if name == "" {
					showError(f.app, f.currentPanel, "Configuration name cannot be empty")
					return
				}
				if err := f.saveConfig(name, config); err != nil {
					showError(f.app, f.currentPanel, fmt.Sprintf("Failed to save config: %v", err))
				} else {
					showMessage(f.app, f.currentPanel, "Configuration saved successfully")
					f.showConfigList() // Refresh the list
				}
			} else {
				f.app.SetRoot(f.currentPanel, true)
			}
		})

	// Create a flex container to hold both the input flex and modal
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	mainFlex.AddItem(flex, 0, 1, true)
	mainFlex.AddItem(modal, 0, 1, false)

	f.app.SetRoot(mainFlex, true)
	f.app.SetFocus(inputField)
}

// saveConfig saves the configuration to a file
func (f *CreateJobForm) saveConfig(name string, config *Config) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, configDir, configListDir, name+".yaml")
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// showConfigList shows the list of available configurations
func (f *CreateJobForm) showConfigList() {
	configs, err := loadConfigList()
	if err != nil {
		showError(f.app, f.flex, fmt.Sprintf("Failed to load configurations: %v", err))
		return
	}

	list := tview.NewList()
	list.SetBorder(true).
		SetTitle("Available Configurations").
		SetTitleAlign(tview.AlignLeft)

	// Add "Create New" option
	list.AddItem("Create New Configuration", "Create a new job configuration", 'n', func() {
		config := &Config{
			User:         os.Getenv("USER"),
			QueueName:    "eidf029ns-user-queue",
			ImageName:    "nvcr.io/nvidia/pytorch:23.12-py3",
			Command:      "apt update && apt install -y tmux && cd ~ && while true; do sleep 60; done;",
			CPUNum:       "24",
			MemoryNum:    "160Gi",
			GPUNum:       "1",
			GPUProduct:   "NVIDIA-H100-80GB-HBM3",
			Mount:        "/root/workspace",
			WorkspacePVC: os.Getenv("USER") + "-ws4",
			NFSPath:      "/",
			NFSServer:    "10.24.1.255",
		}
		form := f.createConfigForm(config)
		f.currentPanel = form
		f.app.SetRoot(form, true)
	})

	// Add existing configurations
	for _, name := range configs {
		configName := name // Create a new variable to avoid closure issues
		list.AddItem(configName, "Press (l) to load, (d) to delete", 'l', func() {
			config, err := loadConfig(configName)
			if err != nil {
				showError(f.app, list, fmt.Sprintf("Failed to load configuration: %v", err))
				return
			}
			form := f.createConfigForm(config)
			f.currentPanel = form
			f.app.SetRoot(form, true)
		})
	}

	// Add exit option
	list.AddItem("Exit", "Return to main view", 'q', f.onClose)

	// Set keyboard shortcuts for the list
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'd':
				// Get the current selection
				index := list.GetCurrentItem()
				if index > 0 && index <= len(configs) { // Skip the "Create New" option and check if it's a valid config
					configName := configs[index-1]
					// Show confirmation dialog
					modal := tview.NewModal().
						SetText(fmt.Sprintf("Are you sure you want to delete configuration '%s'?", configName)).
						AddButtons([]string{"Cancel", "Delete"}).
						SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							if buttonLabel == "Delete" {
								if err := deleteConfig(configName); err != nil {
									showError(f.app, list, fmt.Sprintf("Failed to delete configuration: %v", err))
								} else {
									// Refresh the list
									f.showConfigList()
								}
							} else {
								f.app.SetRoot(list, true)
							}
						})
					f.app.SetRoot(modal, true)
				}
				return nil
			}
		}
		return event
	})

	f.configList = list
	f.currentPanel = list
	f.app.SetRoot(list, true)
}

// NewCreateJobForm creates a new job creation form
func NewCreateJobForm(app *tview.Application, ctx context.Context, onClose func()) *CreateJobForm {
	// Initialize required directories and download base config
	if err := initializeDirectories(); err != nil {
		showError(app, nil, fmt.Sprintf("Failed to initialize directories: %v", err))
		return nil
	}

	if err := downloadBaseConfig(); err != nil {
		showError(app, nil, fmt.Sprintf("Failed to download base config: %v", err))
		return nil
	}

	form := &CreateJobForm{
		app:     app,
		onClose: onClose,
		flex:    tview.NewFlex(),
	}

	// Show the configuration list
	form.showConfigList()

	return form
}

// Show displays the form
func (f *CreateJobForm) Show() {
	f.app.SetRoot(f.currentPanel, true)
}

// GetRoot returns the root primitive of the form
func (f *CreateJobForm) GetRoot() tview.Primitive {
	return f.currentPanel
}

// applyJobConfig applies the job configuration using kubectl and envsubst
func applyJobConfig(config Config) error {
	// Get the path to the base config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	baseConfigPath := filepath.Join(homeDir, configDir, "base_apply.yaml")

	// Create environment variables for envsubst
	env := os.Environ()
	env = append(env, fmt.Sprintf("USER=%s", config.User))
	env = append(env, fmt.Sprintf("QUEUE_NAME=%s", config.QueueName))
	env = append(env, fmt.Sprintf("IMAGE_NAME=%s", config.ImageName))
	env = append(env, fmt.Sprintf("COMMAND=%s", config.Command))
	env = append(env, fmt.Sprintf("CPU_NUM=%s", config.CPUNum))
	env = append(env, fmt.Sprintf("MEMORY_NUM=%s", config.MemoryNum))
	env = append(env, fmt.Sprintf("GPU_NUM=%s", config.GPUNum))
	env = append(env, fmt.Sprintf("GPU_PRODUCT=%s", config.GPUProduct))
	env = append(env, fmt.Sprintf("MOUNT=%s", config.Mount))
	env = append(env, fmt.Sprintf("WORKSPACE_PVC=%s", config.WorkspacePVC))
	env = append(env, fmt.Sprintf("NFS_PATH=%s", config.NFSPath))
	env = append(env, fmt.Sprintf("NFS_SERVER=%s", config.NFSServer))

	// Create the kubectl command with envsubst
	cmd := exec.Command("kubectl", "create", "-f", "-")
	cmd.Env = env

	// Create the envsubst command
	envsubstCmd := exec.Command("envsubst")
	envsubstCmd.Env = env

	// Read the base config file
	baseConfigFile, err := os.Open(baseConfigPath)
	if err != nil {
		return fmt.Errorf("failed to open base config: %w", err)
	}
	defer baseConfigFile.Close()

	// Set up the pipe
	envsubstCmd.Stdin = baseConfigFile
	cmd.Stdin, err = envsubstCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	// Start envsubst
	if err := envsubstCmd.Start(); err != nil {
		return fmt.Errorf("failed to start envsubst: %w", err)
	}

	// Run kubectl
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create job: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// showError displays an error message
func showError(app *tview.Application, root tview.Primitive, message string) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.SetRoot(root, true)
		})
	app.SetRoot(modal, true)
}

// showMessage displays a success message
func showMessage(app *tview.Application, root tview.Primitive, message string) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.SetRoot(root, true)
		})
	app.SetRoot(modal, true)
}

// deleteConfig deletes a configuration file
func deleteConfig(name string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, configDir, configListDir, name+".yaml")
	if err := os.Remove(configPath); err != nil {
		return fmt.Errorf("failed to delete config file: %w", err)
	}

	return nil
}

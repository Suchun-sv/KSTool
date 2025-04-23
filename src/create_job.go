package src

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	EnvVars map[string]string `yaml:"env_vars"`
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
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	// Create .kstool directory
	kstoolDir := filepath.Join(homeDir, configDir)
	if err := os.MkdirAll(kstoolDir, 0755); err != nil {
		return fmt.Errorf("failed to create .kstool directory: %v", err)
	}

	// Create env_config_list directory
	configListPath := filepath.Join(kstoolDir, configListDir)
	if err := os.MkdirAll(configListPath, 0755); err != nil {
		return fmt.Errorf("failed to create env_config_list directory: %v", err)
	}

	return nil
}

// downloadBaseConfig downloads the base configuration file if it doesn't exist
func downloadBaseConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	baseConfigPath := filepath.Join(homeDir, configDir, "base_apply.yaml")
	if _, err := os.Stat(baseConfigPath); err == nil {
		return nil // File exists
	}

	// Download the file
	resp, err := http.Get(baseConfigURL)
	if err != nil {
		return fmt.Errorf("failed to download base config: %v", err)
	}
	defer resp.Body.Close()

	// Read the content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	// Save the original base config
	if err := os.WriteFile(baseConfigPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write base config file: %v", err)
	}

	// Create template file with $VAR_NAME format
	templatePath := filepath.Join(homeDir, configDir, "base_apply_template.yaml")
	re := regexp.MustCompile(`\${([^:}]+):-[^}]+}`)
	processedContent := re.ReplaceAllString(string(content), "$$$1")

	if err := os.WriteFile(templatePath, []byte(processedContent), 0644); err != nil {
		return fmt.Errorf("failed to write template file: %v", err)
	}

	return nil
}

// loadConfigList loads all configuration files from the env_config_list directory
func loadConfigList() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %v", err)
	}

	configListPath := filepath.Join(homeDir, configDir, configListDir)
	files, err := os.ReadDir(configListPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read config list directory: %v", err)
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
		return nil, fmt.Errorf("failed to get home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, configDir, configListDir, name+".yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

// extractEnvVars extracts environment variables and their default values from YAML content
func extractEnvVars(yamlContent []byte) (map[string]string, error) {
	var data map[string]interface{}
	if err := yaml.Unmarshal(yamlContent, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %v", err)
	}

	envVars := make(map[string]string)

	// Function to recursively search for environment variables and their default values
	var searchEnvVars func(interface{})
	searchEnvVars = func(value interface{}) {
		switch v := value.(type) {
		case string:
			// Match pattern ${VAR_NAME:-default_value}
			if matches := regexp.MustCompile(`\${([^:}]+):-([^}]+)}`).FindStringSubmatch(v); len(matches) > 2 {
				envVar := matches[1]
				defaultValue := matches[2]
				if _, exists := envVars[envVar]; !exists {
					envVars[envVar] = defaultValue
				}
			}
		case map[string]interface{}:
			for _, val := range v {
				searchEnvVars(val)
			}
		case []interface{}:
			for _, item := range v {
				searchEnvVars(item)
			}
		}
	}

	searchEnvVars(data)
	return envVars, nil
}

// loadBaseConfig loads the base configuration and extracts environment variables with their default values
func loadBaseConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %v", err)
	}

	baseConfigPath := filepath.Join(homeDir, configDir, "base_apply.yaml")
	data, err := os.ReadFile(baseConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read base config: %v", err)
	}

	envVars, err := extractEnvVars(data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract environment variables: %v", err)
	}

	// Set special default values that can't be extracted from the template
	// if _, exists := envVars["USER"]; exists {
	// 	envVars["USER"] = os.Getenv("USER")
	// }
	// if _, exists := envVars["WORKSPACE_PVC"]; exists {
	// 	envVars["WORKSPACE_PVC"] = os.Getenv("USER") + "-ws4"
	// }

	return &Config{EnvVars: envVars}, nil
}

// createConfigForm creates a form for editing configuration
func (f *CreateJobForm) createConfigForm(config *Config) tview.Primitive {
	// Create a form for all settings
	form := tview.NewForm()
	form.SetBorder(true).SetTitle("Job Configuration").SetTitleAlign(tview.AlignLeft)

	// Track if the form has been modified
	modified := false

	// Add form fields for each environment variable
	for envVar, value := range config.EnvVars {
		// Special handling for GPU product dropdown
		if envVar == "GPU_PRODUCT" {
			gpuOptions := []string{"NVIDIA-H200", "NVIDIA-H100-80GB-HBM3", "NVIDIA-A100-SXM4-80GB", "NVIDIA-A100-SXM4-40GB-MIG-3g.20gb"}
			defaultIndex := 0
			for i, option := range gpuOptions {
				if option == value {
					defaultIndex = i
					break
				}
			}
			form.AddDropDown(envVar, gpuOptions, defaultIndex, func(option string, index int) {
				config.EnvVars[envVar] = option
				modified = true
			})
		} else {
			form.AddInputField(envVar, value, 30, nil, func(text string) {
				config.EnvVars[envVar] = text
				modified = true
			})
		}
	}

	// Function to edit configuration in Vim
	editInVim := func() {
		// Create a temporary file
		tmpFile, err := os.CreateTemp("", "kstool-config-*.yaml")
		if err != nil {
			showError(f.app, form, fmt.Sprintf("Failed to create temporary file: %v", err))
			return
		}
		defer os.Remove(tmpFile.Name())

		// Convert current config to YAML
		yamlData, err := yaml.Marshal(config.EnvVars)
		if err != nil {
			showError(f.app, form, fmt.Sprintf("Failed to convert config to YAML: %v", err))
			return
		}

		// Write current config to the file
		if _, err := tmpFile.Write(yamlData); err != nil {
			showError(f.app, form, fmt.Sprintf("Failed to write to temporary file: %v", err))
			return
		}
		tmpFile.Close()

		// Save the current terminal state
		f.app.Suspend(func() {
			// Launch Vim
			cmd := exec.Command("vim", tmpFile.Name())
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				showError(f.app, form, fmt.Sprintf("Failed to run Vim: %v", err))
				return
			}

			// Read the edited file
			editedData, err := os.ReadFile(tmpFile.Name())
			if err != nil {
				showError(f.app, form, fmt.Sprintf("Failed to read edited file: %v", err))
				return
			}

			// Update the config
			var newEnvVars map[string]string
			if err := yaml.Unmarshal(editedData, &newEnvVars); err != nil {
				showError(f.app, form, fmt.Sprintf("Invalid YAML format: %v", err))
				return
			}

			// Update the form fields
			config.EnvVars = newEnvVars
			var formIndex int
			for envVar, value := range config.EnvVars {
				if envVar == "GPU_PRODUCT" {
					for j, option := range []string{"NVIDIA-H200", "NVIDIA-H100-80GB-HBM3", "NVIDIA-A100-SXM4-80GB", "NVIDIA-A100-SXM4-40GB-MIG-3g.20gb"} {
						if option == value {
							form.GetFormItem(formIndex).(*tview.DropDown).SetCurrentOption(j)
							break
						}
					}
				} else {
					form.GetFormItem(formIndex).(*tview.InputField).SetText(value)
				}
				formIndex++
			}

			modified = true
		})
	}

	// Add buttons
	form.AddButton("Edit in Vim (e)", editInVim)
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

	// Add help text at the bottom
	helpText := tview.NewTextView().
		SetText("Navigation: Mouse Click - Select field | j/k - Move up/down | Tab/Shift+Tab - Next/Previous | e - Edit in Vim | Ctrl+S - Save | F5 - Apply | Esc - Back").
		SetTextAlign(tview.AlignCenter)

	// Create the main layout
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	mainFlex.AddItem(form, 0, 1, true)
	mainFlex.AddItem(helpText, 1, 0, false)

	// Set keyboard shortcuts
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyRune:
			switch event.Rune() {
			case 'j':
				form.SetFocus(form.GetFormItemCount() - 1)
				return nil
			case 'k':
				form.SetFocus(0)
				return nil
			case 'e':
				editInVim()
				return nil
			}
		}
		return event
	})

	return mainFlex
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
	data, err := yaml.Marshal(config.EnvVars)
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
		// Load base config to get default values
		config, err := loadBaseConfig()
		if err != nil {
			showError(f.app, list, fmt.Sprintf("Failed to load base config: %v", err))
			return
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
			// Show action selection dialog
			modal := tview.NewModal().
				SetText(fmt.Sprintf("Configuration: %s\n\nSelect action:", configName)).
				AddButtons([]string{"Apply", "Change", "Back"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					switch buttonLabel {
					case "Apply":
						if err := applyJobConfig(*config); err != nil {
							showError(f.app, list, fmt.Sprintf("Failed to apply job: %v", err))
						} else {
							showMessage(f.app, list, "Job created successfully")
							f.onClose()
						}
					case "Change":
						form := f.createConfigForm(config)
						f.currentPanel = form
						f.app.SetRoot(form, true)
					case "Back":
						f.app.SetRoot(list, true)
					}
				})
			f.app.SetRoot(modal, true)
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
			case 'j':
				// Move to next item
				currentIndex := list.GetCurrentItem()
				if currentIndex < list.GetItemCount()-1 {
					list.SetCurrentItem(currentIndex + 1)
				}
				return nil
			case 'k':
				// Move to previous item
				currentIndex := list.GetCurrentItem()
				if currentIndex > 0 {
					list.SetCurrentItem(currentIndex - 1)
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

	// Load base config
	config, err := loadBaseConfig()
	if err != nil {
		showError(app, nil, fmt.Sprintf("Failed to load base config: %v", err))
		return nil
	}

	// Enable mouse support at the application level
	app.EnableMouse(true)

	form := &CreateJobForm{
		app:     app,
		onClose: onClose,
		flex:    tview.NewFlex(),
		config:  config,
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	templatePath := filepath.Join(homeDir, configDir, "base_apply_template.yaml")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template config: %v", err)
	}

	// Create a temporary file for envsubst
	tempFile, err := os.CreateTemp("", "config_*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("failed to write to temporary file: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %v", err)
	}

	// Set environment variables
	env := os.Environ()
	for key, value := range config.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Run envsubst with the template
	cmd := exec.Command("envsubst")
	cmd.Env = env

	// Read from the template file
	input, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to read template file: %v", err)
	}
	cmd.Stdin = strings.NewReader(string(input))

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run envsubst: %v", err)
	}

	// Write the output to a temporary file
	outputFile, err := os.CreateTemp("", "output_*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer os.Remove(outputFile.Name())

	if _, err := outputFile.Write(output); err != nil {
		return fmt.Errorf("failed to write output: %v", err)
	}
	if err := outputFile.Close(); err != nil {
		return fmt.Errorf("failed to close output file: %v", err)
	}

	// Apply the configuration using kubectl
	applyCmd := exec.Command("kubectl", "apply", "-f", outputFile.Name())
	if output, err := applyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to apply configuration: %v\nOutput: %s", err, output)
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

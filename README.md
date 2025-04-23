# KSTool - Kubernetes Job Management Tool 🚀

<p align="center">
  <img src="KSTool.png" alt="KSTool Logo" width="200"/>
</p>

KSTool is a powerful Terminal User Interface (TUI) application designed to simplify the management of Kubernetes jobs (Specifically For the EIDF users, if you have the general Kubernetes users, I recommend you to use k9s or other more powerful tools). It provides an intuitive interface for creating, managing, and applying job configurations with ease.

## Features 🌟

- **Job Listing and Monitoring** 📊:
  - Real-time job status monitoring
  - Detailed job information display:
    - Job name and status
    - Completion status
    - Duration and age
    - GPU allocation and type
    - Pod status

- **Advanced Filtering** 🔍:
  - Filter by job status:
    - All jobs ✅
    - Running jobs 🟢
    - Failed jobs 🔴
    - Pending jobs ⏳
  - Filter by user:
    - All users 👥
    - Current user 👤

- **Flexible Sorting Options** 📋:
  - Age (newest/oldest first) ⏰
  - GPU count (ascending/descending) 🎮
  - Duration (longest/shortest first) ⌛
  - GPU type (H200/H100/A100) 🖥️

- **Interactive Operations** 🛠️:
  - Delete jobs with confirmation ❌
  - Execute into pod shells 🐚

- **Visual Enhancements** 🎨:
  - Color-coded status indicators:
    - 🟢 Green: Running
    - 🔵 Blue: Complete
    - 🔴 Red: Failed
    - 🟡 Yellow: Suspended
    - ⚪ Gray: Waiting
  - GPU type highlighting:
    - 🟡 Gold: H200
    - 🟣 Purple: H100
    - 🔵 Blue: A100
    - ⚪ Gray: No GPU
  - GPU count color scaling 🌈

- **Keyboard Shortcuts** ⌨️:
  - `r`: Refresh job list 
  - `d`: Delete selected job 
  - `e`: Execute into pod shell 
  - `q`: Quit application 
  - `h`: Toggle user filter 
  - `f`: Change status filter 
  - `s`: Change sort mode 
  - Arrow keys: Navigate job list ⬆️⬇️

## Getting Started 🚀

1. **Quick Installation (Recommended)** 📥:

 1.1 Download the latest release
   ```bash
   # Download the latest release
   wget https://github.com/Suchun-sv/KSTool/releases/latest/download/kstool
   
   # Make it executable
   chmod +x kstool
   
   # Run the application
   ./kstool
   ```

 1.2 Build the application from source (Optional)
   ```bash
   # Clone the repository
   git clone https://github.com/Suchun-sv/KSTool.git
   cd KSTool

   # Build the application
   go build

   # If you meet the incompatible issues with the GCLIB, you can try to use the following command to build the application
   # CGO_ENABLED=0 go build -o kstool main.go
   ```

## New Configuration Creation 🛠️

To provide maximum flexibility for different user needs, KSTool implements a decoupled approach between configuration templates and environment variables. This design allows users to easily customize their job configurations while maintaining reusability.

### Template Configuration 📄

1. **Environment Variable Substitution** ✨

   Transform your static YAML configurations into templates by replacing static values with environment variables. For example:

   ```yaml
   # Before
   image: nvcr.io/nvidia/pytorch:23.12-py3

   # After
   image: ${IMAGE_NAME:-nvcr.io/nvidia/pytorch:23.12-py3}
   ```

   > 💡 **Important**: The correct format is `${VARIABLE_NAME:-DEFAULT_VALUE}`
   > - Must include `:-` (not just `:`)
   > - This syntax allows for default values when variables are unset

2. **Special Variables** 🔑

   KSTool provides special handling for certain variables:

   - **USER Variable** 👤
     ```yaml
     # Will be automatically replaced with actual username
     username: ${USER:-default-user}
     workspace: ${WORKSPACE_PVC:-default-user-ws3}
     ```

   - **GPU_PRODUCT Variable** 🎮
     ```yaml
     # Use dropdown menu to select GPU type
     resources:
       gpu: ${GPU_PRODUCT:-NVIDIA-A100-SXM4-80GB}
     ```
     Supported GPU types:
     - NVIDIA-H200
     - NVIDIA-H100-80GB-HBM3
     - NVIDIA-A100-SXM4-80GB
     - NVIDIA-A100-SXM4-40GB-MIG-3g.20gb

3. **Interactive Configuration** ⚡️

   KSTool provides an intuitive interface for configuration management:

   ```bash
   # Launch KSTool
   ./kstool

   # Navigation
   'n' → Enter configuration menu
   '↑/↓' → Navigate options
   'Enter' → Select option
   ```

   **Key Features:**
   - 📝 Form-based environment variable editing
   - ⌨️ Vim mode for advanced editing (press 'e')
   - 💾 Save configurations for future use
   - ▶️ Direct application to Kubernetes

   > 💡 **Pro Tip**: Use Vim mode ('e') for bulk editing and advanced YAML modifications
   > 💡 **Pro Tip**: 🖱️ Mouse support for navigation

### Setup Instructions 📝

1. **Prepare Your Template**
   - Convert your existing YAML to use environment variables
   - Reference examples in `config/examples/example_1.yaml`
   - Use `config/base_apply.yaml` as a starting point

2. **Install the Template**
   ```bash
   # Copy your template to KSTool's configuration directory
   cp your-config.yaml ~/.kstool/base_apply.yaml
   ```

3. **Create a New Configuration**
   ```bash
   ./kstool
   press `n` to the configuration menu
   Use ⬆️⬇️ to focus on the create new configuration
   You can see the list of your defined environment variables, change them as you want
   ```
   **We also provide the VIM mode for you to edit the configuration file, just press `e` to enter the VIM mode, very useful I think**


### Tips & Best Practices 💡

- Use meaningful variable names that reflect their purpose
- Provide sensible default values for optional parameters
- Leverage the GPU_PRODUCT dropdown to avoid typing long GPU names
- Consider using ChatGPT to help convert your static YAML to template format
- Keep your template well-documented for future reference

### Example Template 📋

```yaml
apiVersion: batch/v1
kind: Job
    spec:
      containers:
      - name: ${CONTAINER_NAME:-pytorch}
        image: ${IMAGE_NAME:-nvcr.io/nvidia/pytorch:23.12-py3}
        resources:
          limits:
            nvidia.com/gpu: ${GPU_COUNT:-1}
            nvidia.com/gpu-product: ${GPU_PRODUCT:-NVIDIA-A100-SXM4-80GB}
        volumeMounts:
        - name: workspace
          mountPath: /workspace
      volumes:
      - name: workspace
        persistentVolumeClaim:
          claimName: ${WORKSPACE_PVC:-default-user-ws3}
```

## Requirements 📋

- Go 1.16 or higher
- Kubernetes cluster access
- kubectl installed and configured
- Vim (for advanced editing)
- envsubst utility

## Configuration Files 📁

The tool manages several types of configuration files:

- `base_apply.yaml`: Base template with default values
- `base_apply_template.yaml`: Template with variable placeholders
- User configurations in `~/.kstool/env_config_list/`

## Contributing 🤝

Contributions are welcome! Please feel free to submit a Pull Request.

## License 📄

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

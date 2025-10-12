#!/bin/bash

set -euo pipefail

# Configuration
REPO_OWNER="alikhil"
REPO_NAME="kubectl-find"
KREW_YAML_FILE="plugins/find.yaml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if required tools are installed
check_dependencies() {
    local missing_deps=()

    if ! command -v curl &> /dev/null; then
        missing_deps+=("curl")
    fi

    if ! command -v jq &> /dev/null; then
        missing_deps+=("jq")
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        print_error "Missing required dependencies: ${missing_deps[*]}"
        print_info "Please install them and try again."
        print_info "  - curl: for API requests"
        print_info "  - jq: for JSON processing"
        exit 1
    fi
}

# Function to get the latest release tag from GitHub
get_latest_release() {
    local api_url="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"

    print_info "Fetching latest release from GitHub..." >&2

    local response
    response=$(curl -s "$api_url")

    if [ $? -ne 0 ]; then
        print_error "Failed to fetch release information from GitHub" >&2
        exit 1
    fi

    local tag_name
    tag_name=$(echo "$response" | jq -r '.tag_name')

    if [ "$tag_name" = "null" ] || [ -z "$tag_name" ]; then
        print_error "Could not parse tag name from GitHub response" >&2
        exit 1
    fi

    print_info "Latest release: $tag_name" >&2
    echo "$tag_name"
}

# Function to download and parse checksums file
get_checksums_from_file() {
    local tag="$1"
    local checksums_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${tag}/kubectl-find_${tag#v}_checksums.txt"

    print_info "Downloading checksums file..." >&2

    local checksums_content
    checksums_content=$(curl -sL "$checksums_url")

    if [ $? -ne 0 ] || [ -z "$checksums_content" ]; then
        print_error "Failed to download checksums file from $checksums_url" >&2
        return 1
    fi

    print_info "Successfully downloaded checksums file" >&2
    echo "$checksums_content"
}

# Function to get checksum for a specific asset from checksums content
get_asset_checksum_from_content() {
    local checksums_content="$1"
    local asset_name="$2"

    local checksum
    checksum=$(echo "$checksums_content" | grep "$asset_name" | cut -d' ' -f1)

    if [ -z "$checksum" ]; then
        print_error "Could not find checksum for $asset_name in checksums file" >&2
        return 1
    fi

    print_info "Checksum for ${asset_name}: $checksum" >&2
    echo "$checksum"
}

# Function to get current version from YAML file using sed
get_current_version() {
    local yaml_file="$1"
    grep "^  version:" "$yaml_file" | sed 's/.*version: *//; s/^"//; s/"$//' | tr -d '\n'
}

# Function to update version in YAML file using sed
update_version_in_yaml() {
    local yaml_file="$1"
    local new_version="$2"

    # Remove any newlines from version
    new_version=$(echo "$new_version" | tr -d '\n')

    # Use sed to replace the version line
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS sed
        sed -i '' "s/^  version: .*/  version: $new_version/" "$yaml_file"
    else
        # Linux sed
        sed -i "s/^  version: .*/  version: $new_version/" "$yaml_file"
    fi
}

# Function to update a specific platform's URI and SHA256
update_platform_in_yaml() {
    local yaml_file="$1"
    local platform_os="$2"
    local platform_arch="$3"
    local new_uri="$4"
    local new_sha256="$5"

    # Create a temporary file for processing
    local temp_file=$(mktemp)
    local in_target_platform=false
    local platform_found=false

    while IFS= read -r line; do
        if [[ "$line" =~ ^[[:space:]]*-[[:space:]]*selector: ]]; then
            # Start of a new platform section
            in_target_platform=false
        elif [[ "$line" =~ ^[[:space:]]*os:[[:space:]]*${platform_os}$ ]]; then
            # Found the target OS
            echo "$line" >> "$temp_file"
            # Check if the next line has the matching architecture
            if IFS= read -r next_line; then
                if [[ "$next_line" =~ ^[[:space:]]*arch:[[:space:]]*${platform_arch}$ ]]; then
                    echo "$next_line" >> "$temp_file"
                    in_target_platform=true
                    platform_found=true
                else
                    echo "$next_line" >> "$temp_file"
                fi
            fi
            continue
        elif [[ "$in_target_platform" == true && "$line" =~ ^[[:space:]]*uri: ]]; then
            # Update the URI
            echo "    uri: $new_uri" >> "$temp_file"
            continue
        elif [[ "$in_target_platform" == true && "$line" =~ ^[[:space:]]*sha256: ]]; then
            # Update the SHA256
            echo "    sha256: $new_sha256" >> "$temp_file"
            continue
        fi

        echo "$line" >> "$temp_file"
    done < "$yaml_file"

    if [[ "$platform_found" == true ]]; then
        mv "$temp_file" "$yaml_file"
        return 0
    else
        rm -f "$temp_file"
        return 1
    fi
}

# Function to update the krew.yaml file
update_krew_yaml() {
    local new_version="$1"

    if [ ! -f "$KREW_YAML_FILE" ]; then
        print_error "krew.yaml file not found in current directory"
        exit 1
    fi

    print_info "Updating krew.yaml with version $new_version..."

    # Create a backup
    cp "$KREW_YAML_FILE" "${KREW_YAML_FILE}.backup"
    print_info "Created backup: ${KREW_YAML_FILE}.backup"

    # Update the version
    update_version_in_yaml "$KREW_YAML_FILE" "$new_version"

    # Download checksums file once
    local checksums_content
    checksums_content=$(get_checksums_from_file "$new_version")

    if [ $? -ne 0 ] || [ -z "$checksums_content" ]; then
        print_error "Failed to get checksums file"
        # Restore backup and exit
        mv "${KREW_YAML_FILE}.backup" "$KREW_YAML_FILE"
        exit 1
    fi

    # Define the platform mappings (os, arch, filename)
    declare -a platforms=(
        "linux:amd64:kubectl-find_Linux_x86_64.tar.gz"
        "linux:arm64:kubectl-find_Linux_arm64.tar.gz"
        "darwin:amd64:kubectl-find_Darwin_x86_64.tar.gz"
        "darwin:arm64:kubectl-find_Darwin_arm64.tar.gz"
        "windows:amd64:kubectl-find_Windows_x86_64.zip"
        "windows:arm64:kubectl-find_Windows_arm64.zip"
    )

    # Update each platform
    for platform_info in "${platforms[@]}"; do
        IFS=':' read -r platform_os platform_arch asset_name <<< "$platform_info"
        local download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${new_version}/${asset_name}"

        print_info "Processing platform: ${platform_os}-${platform_arch}"

        # Get checksum for this asset from the checksums content
        local checksum
        checksum=$(get_asset_checksum_from_content "$checksums_content" "$asset_name")

        if [ $? -ne 0 ] || [ -z "$checksum" ]; then
            print_error "Failed to get checksum for $asset_name"
            # Restore backup and exit
            mv "${KREW_YAML_FILE}.backup" "$KREW_YAML_FILE"
            exit 1
        fi

        # Update the URI and SHA256 for this platform
        if ! update_platform_in_yaml "$KREW_YAML_FILE" "$platform_os" "$platform_arch" "$download_url" "$checksum"; then
            print_error "Failed to update platform ${platform_os}-${platform_arch} in YAML"
            # Restore backup and exit
            mv "${KREW_YAML_FILE}.backup" "$KREW_YAML_FILE"
            exit 1
        fi

        print_info "Updated ${platform_os}-${platform_arch} successfully"
    done

    print_info "Successfully updated krew.yaml"

    # Remove backup if everything went well
    rm -f "${KREW_YAML_FILE}.backup"
}

# Function to show the changes made
show_changes() {
    local old_version="$1"
    local new_version="$2"

    print_info "Changes made:"
    echo "  Version: $old_version -> $new_version"
    echo "  Updated download URLs and checksums for all platforms"
    echo ""
    print_info "To verify the changes, you can run:"
    echo "  git diff $KREW_YAML_FILE"
}

# Main function
main() {
    print_info "Starting krew.yaml update process..."

    # Check dependencies
    check_dependencies

    # Get current version from krew.yaml
    if [ ! -f "$KREW_YAML_FILE" ]; then
        print_error "krew.yaml file not found in current directory"
        exit 1
    fi

    local current_version
    current_version=$(get_current_version "$KREW_YAML_FILE")

    if [ "$current_version" = "null" ] || [ -z "$current_version" ]; then
        print_error "Could not read current version from krew.yaml"
        exit 1
    fi

    print_info "Current version in krew.yaml: $current_version"

    # Get latest release
    local latest_version
    latest_version=$(get_latest_release)

    # Clean up any newlines
    latest_version=$(echo "$latest_version" | tr -d '\n')
    current_version=$(echo "$current_version" | tr -d '\n')

    if [ "$current_version" = "$latest_version" ]; then
        print_info "krew.yaml is already up to date with version $latest_version"
        exit 0
    fi

    print_info "Updating from $current_version to $latest_version"

    # Update the krew.yaml file
    update_krew_yaml "$latest_version"

    # Show changes
    show_changes "$current_version" "$latest_version"

    print_info "Update completed successfully!"
}

# Run main function
main "$@"

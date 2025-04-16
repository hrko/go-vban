#!/bin/bash

# --- Settings ---
# Target OS list
TARGET_OS=("linux" "darwin" "windows")
# Target architecture list
TARGET_ARCH=("amd64" "arm64")

# --- Function Definitions ---
# Function to display usage and exit
usage() {
  echo "Usage: $0 <target_directory> [output_directory]"
  echo "  Cross-compiles the Go program in <target_directory> for various OS and architectures."
  echo "  Supported OS: ${TARGET_OS[*]}"
  echo "  Supported Arch: ${TARGET_ARCH[*]}"
  echo "  Optional: Specify an output directory for the compiled binaries. Default is current directory."
  exit 1
}

# --- Main Process ---

# 1. Check arguments: Ensure the build target directory is specified
if [ -z "$1" ]; then
  echo "Error: Build target directory is not specified." >&2
  usage
fi

TARGET_DIR="$1"

OUTPUT_DIR="${2:-.}"

# 2. Check if the target directory exists
if [ ! -d "$TARGET_DIR" ]; then
  echo "Error: Directory '$TARGET_DIR' not found." >&2
  exit 1
fi

# 3. Check if the output directory exists
if [ ! -d "$OUTPUT_DIR" ]; then
  echo "Error: Output directory '$OUTPUT_DIR' not found." >&2
  exit 1
fi

# 4. Get the program name (from the directory name)
ABS_TARGET_DIR=$(realpath "$TARGET_DIR")
PROGRAM_NAME=$(basename "$ABS_TARGET_DIR")
if [ -z "$PROGRAM_NAME" ]; then
    echo "Error: Failed to retrieve program name (directory name is empty or invalid)." >&2
    exit 1
fi
echo "Program Name: $PROGRAM_NAME (Directory: $ABS_TARGET_DIR)"

echo "Output Directory: $OUTPUT_DIR"
echo "-------------------------------------------"

# 5. Execute cross-compilation
build_count=0
error_count=0

echo "Starting cross-compilation..."

for os in "${TARGET_OS[@]}"; do
  for arch in "${TARGET_ARCH[@]}"; do
    # Generate output file name
    output_name="${PROGRAM_NAME}_${os}_${arch}"
    if [ "$os" = "windows" ]; then
      output_name="${output_name}.exe"
    fi
    output_path="${OUTPUT_DIR}/${output_name}"

    echo "  Building: ${os}/${arch} -> ${output_path}"

    # Set environment variables and execute go build
    # CGO_ENABLED=0 generates binaries that do not depend on libc
    if env CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$output_path" "$TARGET_DIR"; then
      echo "    -> Success: ${output_path}"
      ((build_count++))
    else
      echo "    -> Failed: ${os}/${arch}" >&2
      ((error_count++))
      # Optionally delete incomplete files generated on failure
      rm -f "$output_path"
    fi
  done
done

echo "-------------------------------------------"
echo "Cross-compilation completed."
echo "Success: ${build_count}"
if [ $error_count -gt 0 ]; then
  echo "Failed: ${error_count}" >&2
  exit 1 # Exit with status 1 if there were errors
fi

exit 0

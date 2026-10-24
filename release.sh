#!/bin/bash

# --- Configuration ---
# default_branch: Your main development branch (e.g., main, master)
# remote_name: The name of your remote repository (usually "origin")
# prerelease_suffix: Suffix for pre-releases (e.g., "beta", "rc.1"). Leave empty for stable.
# generate_notes: Set to "true" to automatically generate release notes from commits.
#                 Set to "false" to extract notes from CHANGELOG.md or be prompted for notes.
# notes_file: Path to a file containing release notes (default: CHANGELOG.md).
#             The script will try to extract the relevant section for the version being released.

default_branch="main"
remote_name="origin"
prerelease_suffix="" # e.g., "rc.1" or "beta"
generate_notes="false"
notes_file="CHANGELOG.md"

# --- Helper Functions ---

extract_changelog_for_version() {
    local version="$1"
    local changelog_file="$2"
    
    if [ ! -f "$changelog_file" ]; then
        echo "Warning: Changelog file '$changelog_file' not found."
        return 1
    fi
    
    # Remove 'v' prefix for matching in changelog
    local version_clean="${version#v}"
    
    # Try to find the section for this version in the changelog
    # Look for patterns like [version], [vversion], ## version, ## [version], etc.
    local start_line
    local version_pattern="${version_clean//./\\.}"
    start_line=$(grep -n -E "^##?\s*\[?v?${version_pattern}(\]|$)" "$changelog_file" | head -n1 | cut -d: -f1)
    
    if [ -z "$start_line" ]; then
        echo "Error: Version $version not found in changelog." >&2
        return 1
    fi
    
    # Find the next version section or end of file
    local end_line
    end_line=$(tail -n +$((start_line + 1)) "$changelog_file" | grep -n -E "^##?\s*\[" | head -n1 | cut -d: -f1)
    
    if [ -n "$end_line" ]; then
        end_line=$((start_line + end_line - 1))
        sed -n "${start_line},${end_line}p" "$changelog_file" | head -n -1
    else
        tail -n +$start_line "$changelog_file"
    fi
}

reset_changelog() {
    local changelog_file="$1"
    
    if [ ! -f "$changelog_file" ]; then
        echo "Warning: Changelog file '$changelog_file' not found."
        return 1
    fi
    
    # Create a backup
    cp "$changelog_file" "${changelog_file}.backup"
    
    # Reset to template
    cat > "$changelog_file" << 'EOF'
# Changelog

## [Unreleased] - TBD

### **New Features**
- _No new features yet_

### **Enhancements**
- _No enhancements yet_

### **Bug Fixes**
- _No bug fixes yet_

### **Internal Changes**
- _No internal changes yet_

### **Tests**
- _No test changes yet_

### **Documentation**
- _No documentation changes yet_

---
EOF
    
    echo "Changelog reset to template. Backup saved as ${changelog_file}.backup"
}
get_latest_tag() {
    if ! git fetch --tags "$remote_name" >/dev/null 2>&1; then
        echo "Error: Failed to fetch tags from $remote_name." >&2
        return 1
    fi
    # Get the latest semantic version tag (vX.Y.Z or X.Y.Z)
    git tag --sort=-v:refname | grep -E '^v?[0-9]+\.[0-9]+\.[0-9]+$' | head -n1
}

increment_version() {
    local version="$1"
    local increment_type="$2"
    local current_major current_minor current_patch

    # Remove 'v' prefix if present
    version="${version#v}"

    IFS='.' read -r current_major current_minor current_patch <<< "$version"

    case "$increment_type" in
        patch)
            current_patch=$((current_patch + 1))
            ;;
        minor)
            current_minor=$((current_minor + 1))
            current_patch=0
            ;;
        major)
            current_major=$((current_major + 1))
            current_minor=0
            current_patch=0
            ;;
        *)
            echo "Error: Invalid increment type '$increment_type'. Use 'patch', 'minor', or 'major'."
            exit 1
            ;;
    esac
    echo "v${current_major}.${current_minor}.${current_patch}"
}

# --- Main Script ---

# 1. Prerequisites check
if ! command -v gh &> /dev/null; then
    echo "GitHub CLI 'gh' could not be found. Please install it."
    exit 1
fi
if ! command -v git &> /dev/null; then
    echo "Git 'git' could not be found. Please install it."
    exit 1
fi


# 2. Ensure we are on the default branch and it's clean
echo "Checking Git status..."
if ! git diff --quiet HEAD || ! git diff --cached --quiet HEAD; then
    echo "Your working directory or staging area is not clean. Please commit or stash changes."
    exit 1
fi
if ! git checkout "$default_branch"; then
    echo "Error: Failed to checkout $default_branch."
    exit 1
fi
echo "Pulling latest changes from $remote_name/$default_branch..."
if ! git pull --ff-only "$remote_name" "$default_branch"; then
    echo "Error: Failed to pull $remote_name/$default_branch. Ensure it fast-forwards cleanly."
    exit 1
fi

# 3. Get the latest tag and determine the next version
if ! latest_tag=$(get_latest_tag); then
    exit 1
fi
new_version=""

if [ -z "$latest_tag" ]; then
    echo "No existing semantic version tags found."
    read -r -p "Enter the initial version (e.g., v0.1.0 or 0.1.0): " new_version
    if [[ ! "$new_version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "Error: Invalid version format. Please use (v)X.Y.Z (e.g., v0.1.0)."
        exit 1
    fi
    # Ensure 'v' prefix
    if [[ ! "$new_version" =~ ^v ]]; then
        new_version="v${new_version}"
    fi
else
    echo "Latest tag found: $latest_tag"
    read -r -p "Increment type (patch, minor, major) or specify full version (e.g., v1.2.3): " version_input

    if [[ "$version_input" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        new_version="$version_input"
        if [[ ! "$new_version" =~ ^v ]]; then
            new_version="v${new_version}"
        fi
    elif [[ "$version_input" == "patch" || "$version_input" == "minor" || "$version_input" == "major" ]]; then
        new_version=$(increment_version "$latest_tag" "$version_input")
    else
        echo "Error: Invalid input. Use 'patch', 'minor', 'major', or a full version like 'vX.Y.Z'."
        exit 1
    fi
fi

if [ -n "$prerelease_suffix" ]; then
    new_version="${new_version}-${prerelease_suffix}"
fi

echo "New version will be: $new_version"
read -r -p "Proceed with creating tag and release? (y/N): " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "Aborted."
    exit 0
fi

# 4. Create and push Git tag
echo "Creating Git tag $new_version..."
if git tag "$new_version"; then
    echo "Pushing tag $new_version to $remote_name..."
    if ! git push "$remote_name" "$new_version"; then
        echo "Error: Failed to push tag $new_version to $remote_name." >&2
        exit 1
    fi
else
    echo "Error: Failed to create Git tag. Does it already exist?"
    exit 1
fi

# 5. Create GitHub Release and upload artifacts
echo "Creating GitHub Release for $new_version..."
release_title="Release $new_version"
release_options=()

if [ -n "$prerelease_suffix" ]; then
    release_options+=("--prerelease")
    release_title="${release_title} (${prerelease_suffix})"
fi

if [ "$generate_notes" == "true" ]; then
    release_options+=("--generate-notes")
else
    # Try to extract changelog content for this version
    if [ -n "$notes_file" ] && [ -f "$notes_file" ]; then
        echo "Extracting release notes from $notes_file..."
        temp_notes_file="$(mktemp "${TMPDIR:-/tmp}/release_notes_${new_version}.XXXXXX")" || { echo "Could not create temporary notes file" >&2; exit 1; }
        trap 'rm -f "$temp_notes_file"' EXIT
        if extract_changelog_for_version "$new_version" "$notes_file" > "$temp_notes_file" 2>/dev/null; then
            if [ -s "$temp_notes_file" ]; then
                echo "Found changelog content for $new_version"
                release_options+=("--notes-file" "$temp_notes_file")
            else
                echo "No content found for $new_version in changelog, prompting for notes..."
                rm -f "$temp_notes_file"
                read -r -e -p "Enter release notes (or leave blank to open editor): " custom_notes
                if [ -n "$custom_notes" ]; then
                    release_options+=("--notes" "$custom_notes")
                fi
            fi
        else
            echo "Could not extract changelog content, prompting for notes..."
            read -r -e -p "Enter release notes (or leave blank to open editor): " custom_notes
            if [ -n "$custom_notes" ]; then
                release_options+=("--notes" "$custom_notes")
            fi
        fi
    else
        read -r -e -p "Enter release notes (or leave blank to open editor): " custom_notes
        if [ -n "$custom_notes" ]; then
            release_options+=("--notes" "$custom_notes")
        fi
    fi
fi

if ! gh release create "$new_version" --title "$release_title" "${release_options[@]}"; then
    if ! gh release view "$new_version" >/dev/null 2>&1; then
        echo "Error: Failed to create GitHub Release."
        echo "You might need to: "
        echo "  1. Ensure 'gh' is authenticated with sufficient permissions (repo scope)."
        echo "The tag '$new_version' was pushed. You might need to create the release manually or delete the tag and retry."
        if [ -n "$temp_notes_file" ] && [ -f "$temp_notes_file" ]; then
            rm -f "$temp_notes_file"
        fi
        exit 1
    fi
    echo "Release $new_version was already created by the release workflow; applying release notes."
    if [ -n "$temp_notes_file" ] && [ -s "$temp_notes_file" ] && ! gh release edit "$new_version" --notes-file "$temp_notes_file"; then
        echo "Error: Failed to apply release notes to $new_version." >&2
        exit 1
    fi
fi
echo "GitHub Release $new_version is ready. Binaries are attached by the release workflow."
if [ -n "$temp_notes_file" ] && [ -f "$temp_notes_file" ]; then
    rm -f "$temp_notes_file"
fi
gh release view "$new_version" --web

echo ""
read -r -p "Would you like to reset the changelog to prepare for next development cycle? (y/N): " reset_confirm
if [[ "$reset_confirm" == "y" || "$reset_confirm" == "Y" ]]; then
    if [ -n "$notes_file" ] && [ -f "$notes_file" ]; then
        if reset_changelog "$notes_file"; then
            echo "Changelog has been reset for next development cycle."
        else
            echo "Warning: Failed to reset changelog."
        fi
    else
        echo "Warning: Changelog file not found or not configured."
    fi
fi

echo "Done."
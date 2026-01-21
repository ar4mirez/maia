#!/usr/bin/env bash
#
# MAIA Restore Script
# Restores MAIA data from a backup archive
#
# Usage: ./restore.sh [options] <backup-file>
#
# Options:
#   -d, --data-dir DIR     Target data directory (default: ./data)
#   -t, --tenant TENANT    Restore to specific tenant directory
#   -f, --force            Overwrite existing data without confirmation
#   -k, --keep-existing    Keep existing data, merge with backup
#   -n, --dry-run          Show what would be done without doing it
#   -v, --verbose          Verbose output
#   -h, --help             Show this help message
#
# Environment Variables:
#   MAIA_DATA_DIR         Override default data directory
#   GPG_PASSPHRASE        Passphrase for decrypting GPG-encrypted backups
#
# Examples:
#   # Restore from a backup file
#   ./restore.sh backups/maia_backup_20240115_120000.tar.gz
#
#   # Restore encrypted backup
#   GPG_PASSPHRASE="secret" ./restore.sh backups/maia_backup_20240115_120000.tar.gz.gpg
#
#   # Restore to specific tenant
#   ./restore.sh --tenant acme-corp backups/maia_backup_20240115_120000_tenant_acme-corp.tar
#
#   # Dry run to see what would happen
#   ./restore.sh --dry-run backups/maia_backup_20240115_120000.tar.gz
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
DATA_DIR="${MAIA_DATA_DIR:-./data}"
TENANT=""
FORCE=false
KEEP_EXISTING=false
DRY_RUN=false
VERBOSE=false
BACKUP_FILE=""

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${BLUE}[DEBUG]${NC} $1"
    fi
}

# Show help
show_help() {
    sed -n '/^# Usage:/,/^# Examples:/p' "$0" | head -n -1 | sed 's/^# //'
    echo ""
    sed -n '/^# Examples:/,/^$/p' "$0" | sed 's/^# //'
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -d|--data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            -t|--tenant)
                TENANT="$2"
                shift 2
                ;;
            -f|--force)
                FORCE=true
                shift
                ;;
            -k|--keep-existing)
                KEEP_EXISTING=true
                shift
                ;;
            -n|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            -*)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
            *)
                BACKUP_FILE="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$BACKUP_FILE" ]]; then
        log_error "Backup file is required"
        show_help
        exit 1
    fi
}

# Validate prerequisites
validate_prerequisites() {
    log_verbose "Validating prerequisites..."

    # Check if backup file exists
    if [[ ! -f "$BACKUP_FILE" ]]; then
        log_error "Backup file not found: $BACKUP_FILE"
        exit 1
    fi

    # Determine file type and check for required tools
    if [[ "$BACKUP_FILE" == *.gpg ]]; then
        if ! command -v gpg &> /dev/null; then
            log_error "gpg is required to decrypt this backup but not found"
            exit 1
        fi
    fi

    if [[ "$BACKUP_FILE" == *.gz || "$BACKUP_FILE" == *.tar.gz.gpg ]]; then
        if ! command -v gzip &> /dev/null; then
            log_error "gzip is required to decompress this backup but not found"
            exit 1
        fi
    fi

    log_verbose "Prerequisites validated"
}

# Verify backup checksum
verify_checksum() {
    local checksum_file="${BACKUP_FILE}.sha256"

    if [[ -f "$checksum_file" ]]; then
        log_info "Verifying backup checksum..."
        if sha256sum -c "$checksum_file" &> /dev/null; then
            log_success "Checksum verified"
        else
            log_error "Checksum verification failed!"
            log_error "The backup file may be corrupted"
            exit 1
        fi
    else
        log_warn "No checksum file found, skipping verification"
    fi
}

# Decrypt backup if needed
decrypt_backup() {
    local input_file="$1"

    if [[ "$input_file" == *.gpg ]]; then
        local output_file="${input_file%.gpg}"
        log_info "Decrypting backup..."

        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would decrypt $input_file to $output_file"
            echo "$output_file"
            return
        fi

        if [[ -n "${GPG_PASSPHRASE:-}" ]]; then
            echo "$GPG_PASSPHRASE" | gpg --batch --yes --passphrase-fd 0 --decrypt "$input_file" > "$output_file"
        else
            gpg --batch --yes --decrypt "$input_file" > "$output_file"
        fi

        log_verbose "Decrypted to: $output_file"
        echo "$output_file"
    else
        echo "$input_file"
    fi
}

# Decompress backup if needed
decompress_backup() {
    local input_file="$1"

    if [[ "$input_file" == *.gz ]]; then
        local output_file="${input_file%.gz}"
        log_info "Decompressing backup..."

        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would decompress $input_file to $output_file"
            echo "$output_file"
            return
        fi

        gzip -dk "$input_file"
        log_verbose "Decompressed to: $output_file"
        echo "$output_file"
    else
        echo "$input_file"
    fi
}

# Confirm restore
confirm_restore() {
    if [[ "$FORCE" == "true" ]]; then
        return 0
    fi

    if [[ -d "$DATA_DIR" ]] && [[ "$(ls -A "$DATA_DIR" 2>/dev/null)" ]]; then
        log_warn "Target directory contains existing data: $DATA_DIR"

        if [[ "$KEEP_EXISTING" == "true" ]]; then
            log_info "Backup will be merged with existing data"
        else
            log_warn "Existing data will be OVERWRITTEN!"
        fi

        echo -n "Continue? [y/N] "
        read -r response
        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            log_info "Restore cancelled by user"
            exit 0
        fi
    fi
}

# Perform restore
perform_restore() {
    local tar_file="$1"
    local target_dir="$DATA_DIR"

    if [[ -n "$TENANT" ]]; then
        target_dir="${DATA_DIR}/tenants/${TENANT}"
    fi

    log_info "Restoring backup to: $target_dir"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would extract $tar_file to $target_dir"
        log_info "[DRY-RUN] Archive contents:"
        tar -tvf "$tar_file" | head -20
        return
    fi

    # Create target directory if needed
    mkdir -p "$target_dir"

    # Clear existing data if not keeping
    if [[ "$KEEP_EXISTING" != "true" ]] && [[ -d "$target_dir" ]]; then
        log_verbose "Clearing existing data..."
        rm -rf "${target_dir:?}/"*
    fi

    # Extract backup
    if [[ -n "$TENANT" ]]; then
        # Extracting tenant-specific backup
        tar -xf "$tar_file" -C "${DATA_DIR}/tenants" --strip-components=0
    else
        # Extracting full backup
        tar -xf "$tar_file" -C "$(dirname "$target_dir")" --strip-components=0
    fi

    log_verbose "Extraction complete"

    # Set correct permissions
    chmod -R 755 "$target_dir"

    log_success "Restore completed!"
    log_info "  Target: $target_dir"

    # Show restored files count
    local file_count
    file_count=$(find "$target_dir" -type f | wc -l)
    log_info "  Files restored: $file_count"
}

# Cleanup temporary files
cleanup() {
    local temp_file="$1"
    local original_file="$2"

    # Only clean up files we created during decrypt/decompress
    if [[ "$temp_file" != "$original_file" ]] && [[ -f "$temp_file" ]]; then
        if [[ "$DRY_RUN" != "true" ]]; then
            rm -f "$temp_file"
            log_verbose "Cleaned up temporary file: $temp_file"
        fi
    fi
}

# List backup contents
list_contents() {
    local file="$1"
    log_info "Backup contents (first 20 entries):"
    tar -tvf "$file" 2>/dev/null | head -20 || true
}

# Main function
main() {
    parse_args "$@"

    log_info "MAIA Restore Script"
    log_info "==================="

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "Running in DRY-RUN mode - no changes will be made"
    fi

    validate_prerequisites
    verify_checksum
    confirm_restore

    # Process backup file (decrypt, decompress)
    local working_file="$BACKUP_FILE"
    local decrypted_file=""
    local decompressed_file=""

    # Decrypt if encrypted
    if [[ "$working_file" == *.gpg ]]; then
        decrypted_file=$(decrypt_backup "$working_file")
        working_file="$decrypted_file"
    fi

    # Decompress if compressed
    if [[ "$working_file" == *.gz ]]; then
        decompressed_file=$(decompress_backup "$working_file")
        working_file="$decompressed_file"
    fi

    # Show contents if verbose
    if [[ "$VERBOSE" == "true" ]] && [[ "$DRY_RUN" != "true" ]]; then
        list_contents "$working_file"
    fi

    # Perform restore
    perform_restore "$working_file"

    # Cleanup temporary files
    if [[ -n "$decompressed_file" ]]; then
        cleanup "$decompressed_file" "$BACKUP_FILE"
    fi
    if [[ -n "$decrypted_file" ]]; then
        cleanup "$decrypted_file" "$BACKUP_FILE"
    fi

    log_success "Restore completed successfully!"
}

main "$@"

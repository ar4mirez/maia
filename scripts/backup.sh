#!/usr/bin/env bash
#
# MAIA Backup Script
# Creates timestamped backups of MAIA data with optional compression and encryption
#
# Usage: ./backup.sh [options]
#
# Options:
#   -d, --data-dir DIR     Source data directory (default: ./data)
#   -o, --output-dir DIR   Backup destination directory (default: ./backups)
#   -c, --compress         Compress backup with gzip
#   -e, --encrypt          Encrypt backup with GPG (requires GPG_RECIPIENT)
#   -r, --retention DAYS   Delete backups older than DAYS (default: 30)
#   -t, --tenant TENANT    Backup specific tenant only
#   -n, --dry-run          Show what would be done without doing it
#   -v, --verbose          Verbose output
#   -h, --help             Show this help message
#
# Environment Variables:
#   MAIA_DATA_DIR         Override default data directory
#   MAIA_BACKUP_DIR       Override default backup directory
#   GPG_RECIPIENT         GPG key ID or email for encryption
#   BACKUP_RETENTION_DAYS Override default retention period
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
BACKUP_DIR="${MAIA_BACKUP_DIR:-./backups}"
COMPRESS=false
ENCRYPT=false
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
TENANT=""
DRY_RUN=false
VERBOSE=false
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

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
    sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# //'
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
            -o|--output-dir)
                BACKUP_DIR="$2"
                shift 2
                ;;
            -c|--compress)
                COMPRESS=true
                shift
                ;;
            -e|--encrypt)
                ENCRYPT=true
                shift
                ;;
            -r|--retention)
                RETENTION_DAYS="$2"
                shift 2
                ;;
            -t|--tenant)
                TENANT="$2"
                shift 2
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
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Validate prerequisites
validate_prerequisites() {
    log_verbose "Validating prerequisites..."

    # Check if data directory exists
    if [[ ! -d "$DATA_DIR" ]]; then
        log_error "Data directory not found: $DATA_DIR"
        exit 1
    fi

    # Check for required tools
    if [[ "$COMPRESS" == "true" ]] && ! command -v gzip &> /dev/null; then
        log_error "gzip is required for compression but not found"
        exit 1
    fi

    if [[ "$ENCRYPT" == "true" ]]; then
        if ! command -v gpg &> /dev/null; then
            log_error "gpg is required for encryption but not found"
            exit 1
        fi
        if [[ -z "${GPG_RECIPIENT:-}" ]]; then
            log_error "GPG_RECIPIENT environment variable is required for encryption"
            exit 1
        fi
    fi

    log_verbose "Prerequisites validated"
}

# Create backup directory
create_backup_dir() {
    if [[ ! -d "$BACKUP_DIR" ]]; then
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY-RUN] Would create backup directory: $BACKUP_DIR"
        else
            mkdir -p "$BACKUP_DIR"
            log_verbose "Created backup directory: $BACKUP_DIR"
        fi
    fi
}

# Generate backup filename
generate_backup_name() {
    local name="maia_backup_${TIMESTAMP}"

    if [[ -n "$TENANT" ]]; then
        name="${name}_tenant_${TENANT}"
    fi

    echo "$name"
}

# Create backup
create_backup() {
    local backup_name
    backup_name=$(generate_backup_name)
    local backup_path="${BACKUP_DIR}/${backup_name}.tar"

    log_info "Starting backup..."
    log_verbose "Source: $DATA_DIR"
    log_verbose "Destination: $BACKUP_DIR"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would create backup: $backup_path"
    else
        # Create tar archive
        if [[ -n "$TENANT" ]]; then
            # Backup specific tenant directory
            local tenant_dir="${DATA_DIR}/tenants/${TENANT}"
            if [[ ! -d "$tenant_dir" ]]; then
                log_error "Tenant directory not found: $tenant_dir"
                exit 1
            fi
            tar -cf "$backup_path" -C "${DATA_DIR}/tenants" "$TENANT"
        else
            # Backup entire data directory
            tar -cf "$backup_path" -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")"
        fi

        log_verbose "Created tar archive: $backup_path"

        # Compress if requested
        if [[ "$COMPRESS" == "true" ]]; then
            gzip "$backup_path"
            backup_path="${backup_path}.gz"
            log_verbose "Compressed backup: $backup_path"
        fi

        # Encrypt if requested
        if [[ "$ENCRYPT" == "true" ]]; then
            gpg --batch --yes --recipient "${GPG_RECIPIENT}" --encrypt "$backup_path"
            rm -f "$backup_path"
            backup_path="${backup_path}.gpg"
            log_verbose "Encrypted backup: $backup_path"
        fi

        # Calculate and store checksum
        local checksum_file="${backup_path}.sha256"
        sha256sum "$backup_path" > "$checksum_file"
        log_verbose "Created checksum: $checksum_file"

        # Get backup size
        local size
        size=$(du -h "$backup_path" | cut -f1)

        log_success "Backup created successfully!"
        log_info "  File: $backup_path"
        log_info "  Size: $size"
        log_info "  Checksum: $checksum_file"
    fi
}

# Clean old backups
cleanup_old_backups() {
    if [[ "$RETENTION_DAYS" -le 0 ]]; then
        log_verbose "Retention disabled, skipping cleanup"
        return
    fi

    log_info "Cleaning up backups older than $RETENTION_DAYS days..."

    local count=0

    if [[ "$DRY_RUN" == "true" ]]; then
        count=$(find "$BACKUP_DIR" -name "maia_backup_*.tar*" -type f -mtime +"$RETENTION_DAYS" | wc -l)
        log_info "[DRY-RUN] Would delete $count old backup(s)"
    else
        while IFS= read -r -d '' file; do
            rm -f "$file" "${file}.sha256" 2>/dev/null || true
            count=$((count + 1))
            log_verbose "Deleted: $file"
        done < <(find "$BACKUP_DIR" -name "maia_backup_*.tar*" -type f -mtime +"$RETENTION_DAYS" -print0)

        if [[ $count -gt 0 ]]; then
            log_info "Deleted $count old backup(s)"
        else
            log_verbose "No old backups to clean up"
        fi
    fi
}

# List existing backups
list_backups() {
    log_info "Existing backups in $BACKUP_DIR:"
    if [[ -d "$BACKUP_DIR" ]]; then
        find "$BACKUP_DIR" -name "maia_backup_*.tar*" -type f -exec ls -lh {} \; | sort -k9
    else
        log_info "  No backups found"
    fi
}

# Main function
main() {
    parse_args "$@"

    log_info "MAIA Backup Script"
    log_info "=================="

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "Running in DRY-RUN mode - no changes will be made"
    fi

    validate_prerequisites
    create_backup_dir
    create_backup
    cleanup_old_backups

    if [[ "$VERBOSE" == "true" ]]; then
        list_backups
    fi

    log_success "Backup completed!"
}

main "$@"

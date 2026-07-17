#!/usr/bin/env bash
set -Eeuo pipefail

if ((BASH_VERSINFO[0] < 4)); then
  printf 'Error: Bash 4 or newer is required; found Bash %s.\n' "$BASH_VERSION" >&2
  exit 1
fi

IMAGE=${IMAGE:-pczhao1210/caddy-reverse-proxy:latest}
DEPLOY_MODE=${DEPLOY_MODE:-}
DEFAULT_LOCATION=${LOCATION:-japaneast}
DEFAULT_VM_NAME=${VM_NAME:-caddy-gateway}
DEFAULT_RESOURCE_GROUP=${RESOURCE_GROUP:-rg-caddy-gateway}
DEFAULT_ADMIN_USERNAME=${ADMIN_USERNAME:-azureuser}
UBUNTU_IMAGE=${UBUNTU_IMAGE:-Canonical:ubuntu-24_04-lts:server:latest}
ROLLBACK_ON_ERROR=${ROLLBACK_ON_ERROR:-true}
DEPLOY_SCRIPT_BASE_URL=${DEPLOY_SCRIPT_BASE_URL:-https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main}
LOCAL_INSTALL_DIR=${LOCAL_INSTALL_DIR:-$HOME/caddy-reverse-proxy}
LOCAL_DATA_DIR=${DATA_DIR:-$HOME/docker_files/caddy-reverse-proxy}
LOCAL_CONTAINER_NAME=${CONTAINER_NAME:-caddy-reverse-proxy}
LOCAL_HTTP_PORT=${HTTP_PORT:-80}
LOCAL_HTTPS_PORT=${HTTPS_PORT:-443}
LOCAL_MANAGEMENT_PORT=${MANAGEMENT_PORT:-8080}
LOCAL_DOCKER_NETWORKS=${DOCKER_NETWORKS:-}
SSH_PUBLIC_KEY_SOURCE=${SSH_PUBLIC_KEY_SOURCE:-}
SSH_PUBLIC_KEY_FILE=${SSH_PUBLIC_KEY_FILE:-}
SSH_PUBLIC_KEY_RESOURCE_NAME=${SSH_PUBLIC_KEY_RESOURCE_NAME:-}
SSH_PUBLIC_KEY_RESOURCE_GROUP=${SSH_PUBLIC_KEY_RESOURCE_GROUP:-}
SSH_KEY_VAULT_NAME=${SSH_KEY_VAULT_NAME:-}
SSH_KEY_VAULT_SECRET_NAME=${SSH_KEY_VAULT_SECRET_NAME:-}
SSH_KEY_VAULT_SECRET_VERSION=${SSH_KEY_VAULT_SECRET_VERSION:-}
VM_AUTHENTICATION_TYPE=${VM_AUTHENTICATION_TYPE:-}
SSH_PRIVATE_KEY_FILE=${SSH_PRIVATE_KEY_FILE:-}
SSH_PUBLIC_KEY_DESCRIPTION=
VM_AUTHENTICATION_DESCRIPTION=
GENERATE_SSH_KEY=false
VM_AUTHENTICATION_ARGS=()

TEMP_FILES=()
TEMP_DIRS=()
SCRIPT_PID=$BASHPID
DEPLOYMENT_STARTED=false
DEPLOYMENT_SUCCEEDED=false
VM_CREATE_ATTEMPTED=false
MANAGED_NIC=false
MANAGED_PUBLIC_IP=false
MANAGED_NSG=false
CREATED_VNET=false
CREATED_SUBNET=false

cleanup() {
  local path
  for path in "${TEMP_FILES[@]:-}"; do
    [[ -n "$path" ]] && rm -f -- "$path"
  done
  for path in "${TEMP_DIRS[@]:-}"; do
    [[ -n "$path" ]] && rm -rf -- "$path"
  done
}

rollback_resources() {
  local remaining=()
  set +e
  printf '\nDeployment failed; removing resources created by this run...\n' >&2

  if [[ "$VM_CREATE_ATTEMPTED" == true ]]; then
    az vm delete --resource-group "$RESOURCE_GROUP" --name "$VM_NAME" --yes --output none >/dev/null 2>&1
    az disk delete --resource-group "$RESOURCE_GROUP" --name "$OS_DISK_NAME" --yes --output none >/dev/null 2>&1
  fi
  if [[ "$MANAGED_NIC" == true ]]; then
    az network nic delete --resource-group "$RESOURCE_GROUP" --name "$NIC_NAME" >/dev/null 2>&1
  fi
  if [[ "$MANAGED_PUBLIC_IP" == true ]]; then
    az network public-ip delete --resource-group "$RESOURCE_GROUP" --name "$PUBLIC_IP_NAME" >/dev/null 2>&1
  fi
  if [[ "$MANAGED_NSG" == true ]]; then
    az network nsg delete --resource-group "$RESOURCE_GROUP" --name "$NSG_NAME" >/dev/null 2>&1
  fi
  if [[ "$CREATED_SUBNET" == true ]]; then
    az network vnet subnet delete \
      --resource-group "$VNET_RESOURCE_GROUP" \
      --vnet-name "$VNET_NAME" \
      --name "$SUBNET_NAME" >/dev/null 2>&1
  fi
  if [[ "$CREATED_VNET" == true ]]; then
    az network vnet delete \
      --resource-group "$RESOURCE_GROUP" \
      --name "$VNET_NAME" >/dev/null 2>&1
  fi

  if [[ "$VM_CREATE_ATTEMPTED" == true ]]; then
    az vm show --resource-group "$RESOURCE_GROUP" --name "$VM_NAME" >/dev/null 2>&1 && remaining+=("VM $VM_NAME")
    az disk show --resource-group "$RESOURCE_GROUP" --name "$OS_DISK_NAME" >/dev/null 2>&1 && remaining+=("disk $OS_DISK_NAME")
  fi
  if [[ "$MANAGED_NIC" == true ]] && az network nic show --resource-group "$RESOURCE_GROUP" --name "$NIC_NAME" >/dev/null 2>&1; then
    remaining+=("NIC $NIC_NAME")
  fi
  if [[ "$MANAGED_PUBLIC_IP" == true ]] && az network public-ip show --resource-group "$RESOURCE_GROUP" --name "$PUBLIC_IP_NAME" >/dev/null 2>&1; then
    remaining+=("public IP $PUBLIC_IP_NAME")
  fi
  if [[ "$MANAGED_NSG" == true ]] && az network nsg show --resource-group "$RESOURCE_GROUP" --name "$NSG_NAME" >/dev/null 2>&1; then
    remaining+=("NSG $NSG_NAME")
  fi
  if [[ "$CREATED_SUBNET" == true ]] && az network vnet subnet show \
    --resource-group "$VNET_RESOURCE_GROUP" \
    --vnet-name "$VNET_NAME" \
    --name "$SUBNET_NAME" >/dev/null 2>&1; then
    remaining+=("subnet $VNET_NAME/$SUBNET_NAME")
  fi
  if [[ "$CREATED_VNET" == true ]] && az network vnet show --resource-group "$RESOURCE_GROUP" --name "$VNET_NAME" >/dev/null 2>&1; then
    remaining+=("VNet $VNET_NAME")
  fi

  if ((${#remaining[@]} > 0)); then
    printf 'Warning: rollback could not remove: %s\n' "${remaining[*]}" >&2
  else
    printf 'Rollback finished. Pre-existing resource groups, VNets, and subnets were not removed.\n' >&2
  fi
}

abort_deployment() {
  local exit_code=$1

  if [[ "$BASHPID" != "$SCRIPT_PID" ]]; then
    exit "$exit_code"
  fi

  trap - ERR INT TERM
  if [[ "$DEPLOYMENT_STARTED" == true && "$DEPLOYMENT_SUCCEEDED" != true ]]; then
    if [[ "$ROLLBACK_ON_ERROR" == true ]]; then
      rollback_resources
    else
      printf '\nDeployment stopped with exit code %s. ROLLBACK_ON_ERROR=false, so created resources were left in place.\n' "$exit_code" >&2
    fi
  fi
  exit "$exit_code"
}

trap cleanup EXIT
trap 'abort_deployment $?' ERR
trap 'abort_deployment 130' INT TERM

fail() {
  printf 'Error: %s\n' "$*" >&2
  abort_deployment 1
}

if [[ -r /dev/tty ]]; then
  INPUT_DEVICE=/dev/tty
elif [[ -t 0 ]]; then
  INPUT_DEVICE=/dev/stdin
else
  fail "Interactive input is required. Run this script from a terminal."
fi

prompt_value() {
  local label=$1
  local default_value=${2:-}
  local value

  if [[ -n "$default_value" ]]; then
    printf '%s [%s]: ' "$label" "$default_value" >&2
  else
    printf '%s: ' "$label" >&2
  fi
  IFS= read -r value <"$INPUT_DEVICE"
  printf '%s\n' "${value:-$default_value}"
}

select_index() {
  local title=$1
  local default_index=$2
  shift 2
  local options=("$@")
  local index answer

  ((${#options[@]} > 0)) || fail "No options are available for $title."
  printf '\n%s\n' "$title" >&2
  for index in "${!options[@]}"; do
    printf '  %d) %s\n' "$((index + 1))" "${options[$index]}" >&2
  done

  while :; do
    answer=$(prompt_value "Select 1-${#options[@]}" "$((default_index + 1))")
    if [[ "$answer" =~ ^[0-9]+$ ]] && ((answer >= 1 && answer <= ${#options[@]})); then
      printf '%s\n' "$((answer - 1))"
      return
    fi
    printf 'Enter a number between 1 and %d.\n' "${#options[@]}" >&2
  done
}

confirm() {
  local answer
  answer=$(prompt_value "$1 (yes/no)" "no")
  [[ "${answer,,}" == "yes" || "${answer,,}" == "y" ]]
}

choose_deployment_mode() {
  local selected_index normalized_mode=${DEPLOY_MODE,,}

  case "$normalized_mode" in
    azure|azure-vm|new)
      printf 'azure-vm\n'
      return
      ;;
    local|container|local-container)
      printf 'local\n'
      return
      ;;
    "") ;;
    *) fail "DEPLOY_MODE must be azure-vm or local." ;;
  esac

  selected_index=$(select_index "Deployment mode" 0 \
    "Create a standalone Azure VM and deploy the gateway" \
    "Deploy only the gateway container on this machine")
  if ((selected_index == 0)); then
    printf 'azure-vm\n'
  else
    printf 'local\n'
  fi
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required."
}

validate_resource_name() {
  local kind=$1
  local value=$2
  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$ ]] || fail "$kind contains unsupported characters or is too long: $value"
}

validate_admin_username() {
  local value=$1
  [[ "$value" =~ ^[a-z_][a-z0-9_-]{0,31}$ ]] || fail "Linux admin username must start with a lowercase letter or underscore and contain at most 32 lowercase letters, digits, underscores, or hyphens."
  case "$value" in
    root|admin|administrator) fail "Linux admin username is reserved by Azure: $value" ;;
  esac
}

validate_ssh_public_key_file() {
  local input_file=$1 key_type

  case "$input_file" in
    ~/*) input_file="$HOME/${input_file#\~/}" ;;
  esac
  [[ -f "$input_file" && -r "$input_file" ]] || fail "SSH public key file is not readable: $input_file"
  input_file=$(realpath -e -- "$input_file")
  if ! read -r key_type _ <"$input_file"; then
    fail "SSH public key file is empty: $input_file"
  fi
  case "$key_type" in
    ssh-*|ecdsa-*|sk-*) ;;
    *) fail "File does not contain an OpenSSH public key: $input_file" ;;
  esac
  ssh-keygen -l -f "$input_file" >/dev/null 2>&1 || fail "Invalid SSH public key file: $input_file"
  SSH_PUBLIC_KEY_FILE=$input_file
}

choose_local_ssh_public_key() {
  local candidate selected_index
  local key_files=() key_labels=()

  if [[ -n "$SSH_PUBLIC_KEY_FILE" ]]; then
    validate_ssh_public_key_file "$SSH_PUBLIC_KEY_FILE"
    SSH_PUBLIC_KEY_DESCRIPTION="local file $SSH_PUBLIC_KEY_FILE"
    return
  fi

  if [[ -d "$HOME/.ssh" ]]; then
    while IFS= read -r -d '' candidate; do
      key_files+=("$candidate")
      key_labels+=("$candidate")
    done < <(find "$HOME/.ssh" -maxdepth 1 -type f -name '*.pub' -print0)
  fi
  key_labels+=("Enter another existing SSH public key file")

  selected_index=$(select_index "Existing SSH public key" 0 "${key_labels[@]}")
  if ((selected_index == ${#key_files[@]})); then
    candidate=$(prompt_value "Existing SSH public key file path")
    [[ -n "$candidate" ]] || fail "An existing SSH public key file is required."
  else
    candidate=${key_files[$selected_index]}
  fi
  validate_ssh_public_key_file "$candidate"
  SSH_PUBLIC_KEY_DESCRIPTION="local file $SSH_PUBLIC_KEY_FILE"
}

choose_azure_ssh_public_key() {
  local index name resource_group location selected_index key_output key_file
  local key_records=() key_labels=() key_names=() key_resource_groups=()

  if [[ -z "$SSH_PUBLIC_KEY_RESOURCE_NAME" || -z "$SSH_PUBLIC_KEY_RESOURCE_GROUP" ]]; then
    key_output=$(az sshkey list --query '[].[name,resourceGroup,location]' -o tsv) || \
      fail "Azure could not list SSH public key resources. Set SSH_PUBLIC_KEY_RESOURCE_NAME and SSH_PUBLIC_KEY_RESOURCE_GROUP explicitly if list permission is unavailable."

    while IFS=$'\t' read -r name resource_group location; do
      [[ -n "$name" ]] || continue
      [[ -n "$SSH_PUBLIC_KEY_RESOURCE_NAME" && "$name" != "$SSH_PUBLIC_KEY_RESOURCE_NAME" ]] && continue
      [[ -n "$SSH_PUBLIC_KEY_RESOURCE_GROUP" && "$resource_group" != "$SSH_PUBLIC_KEY_RESOURCE_GROUP" ]] && continue
      key_records+=("$name"$'\t'"$resource_group"$'\t'"$location")
    done <<<"$key_output"

    ((${#key_records[@]} > 0)) || fail \
      "No Azure SSH public key resource matched the requested name and resource group in the selected subscription."

    selected_index=0
    for index in "${!key_records[@]}"; do
      IFS=$'\t' read -r name resource_group location <<<"${key_records[$index]}"
      key_labels+=("$name (resource group: $resource_group, region: $location)")
      key_names+=("$name")
      key_resource_groups+=("$resource_group")
      [[ "$location" == "$LOCATION" ]] && selected_index=$index
    done

    if ((${#key_records[@]} > 1)); then
      selected_index=$(select_index "SSH public key stored in Azure" "$selected_index" "${key_labels[@]}")
    else
      selected_index=0
    fi
    SSH_PUBLIC_KEY_RESOURCE_NAME=${key_names[$selected_index]}
    SSH_PUBLIC_KEY_RESOURCE_GROUP=${key_resource_groups[$selected_index]}
  fi

  key_file=$(mktemp)
  TEMP_FILES+=("$key_file")
  az sshkey show \
    --resource-group "$SSH_PUBLIC_KEY_RESOURCE_GROUP" \
    --name "$SSH_PUBLIC_KEY_RESOURCE_NAME" \
    --query publicKey \
    -o tsv >"$key_file" || fail \
      "Unable to read Azure SSH public key resource $SSH_PUBLIC_KEY_RESOURCE_GROUP/$SSH_PUBLIC_KEY_RESOURCE_NAME."
  chmod 0600 "$key_file"
  validate_ssh_public_key_file "$key_file"
  SSH_PUBLIC_KEY_DESCRIPTION="Azure SSH public key resource $SSH_PUBLIC_KEY_RESOURCE_GROUP/$SSH_PUBLIC_KEY_RESOURCE_NAME"
}

choose_key_vault_ssh_public_key() {
  local index name resource_group location selected_index secret_name secret_output key_file
  local vault_output
  local secret_args=()
  local vault_records=() vault_labels=() vault_names=()
  local secret_names=()

  if [[ -z "$SSH_KEY_VAULT_NAME" ]]; then
    vault_output=$(az keyvault list --query '[].[name,resourceGroup,location]' -o tsv) || \
      fail "Azure could not list Key Vaults. Set SSH_KEY_VAULT_NAME explicitly if list permission is unavailable."
    mapfile -t vault_records < <(printf '%s\n' "$vault_output" | sed '/^$/d')
    ((${#vault_records[@]} > 0)) || fail "No Azure Key Vault is visible to the current identity."

    selected_index=0
    for index in "${!vault_records[@]}"; do
      IFS=$'\t' read -r name resource_group location <<<"${vault_records[$index]}"
      vault_labels+=("$name (resource group: $resource_group, region: $location)")
      vault_names+=("$name")
      [[ "$location" == "$LOCATION" ]] && selected_index=$index
    done
    selected_index=$(select_index "Azure Key Vault containing the SSH public key secret" "$selected_index" "${vault_labels[@]}")
    SSH_KEY_VAULT_NAME=${vault_names[$selected_index]}
  fi

  if [[ -z "$SSH_KEY_VAULT_SECRET_NAME" ]]; then
    if secret_output=$(az keyvault secret list \
      --vault-name "$SSH_KEY_VAULT_NAME" \
      --query '[?attributes.enabled].name' \
      -o tsv 2>/dev/null); then
      mapfile -t secret_names < <(printf '%s\n' "$secret_output" | sed '/^$/d')
    else
      printf 'Secret list permission is unavailable for %s; enter a known secret name.\n' "$SSH_KEY_VAULT_NAME" >&2
    fi

    if ((${#secret_names[@]} > 0)); then
      selected_index=$(select_index "Key Vault secret containing one OpenSSH public key" 0 "${secret_names[@]}")
      SSH_KEY_VAULT_SECRET_NAME=${secret_names[$selected_index]}
    else
      secret_name=$(prompt_value "Key Vault secret name containing one OpenSSH public key")
      [[ -n "$secret_name" ]] || fail "A Key Vault secret name is required."
      SSH_KEY_VAULT_SECRET_NAME=$secret_name
    fi
  fi

  key_file=$(mktemp)
  TEMP_FILES+=("$key_file")
  secret_args=(
    --vault-name "$SSH_KEY_VAULT_NAME"
    --name "$SSH_KEY_VAULT_SECRET_NAME"
  )
  if [[ -n "$SSH_KEY_VAULT_SECRET_VERSION" ]]; then
    secret_args+=(--version "$SSH_KEY_VAULT_SECRET_VERSION")
  fi
  az keyvault secret show \
    "${secret_args[@]}" \
    --query value \
    -o tsv >"$key_file" || fail \
      "Unable to read Key Vault secret $SSH_KEY_VAULT_NAME/$SSH_KEY_VAULT_SECRET_NAME. Grant secret get permission to the current identity."
  chmod 0600 "$key_file"
  validate_ssh_public_key_file "$key_file"
  SSH_PUBLIC_KEY_DESCRIPTION="Key Vault secret $SSH_KEY_VAULT_NAME/$SSH_KEY_VAULT_SECRET_NAME"
}

choose_ssh_public_key() {
  local selected_index source=${SSH_PUBLIC_KEY_SOURCE,,}

  if [[ -n "$SSH_PUBLIC_KEY_RESOURCE_NAME" || -n "$SSH_PUBLIC_KEY_RESOURCE_GROUP" ]]; then
    source=azure
  elif [[ -n "$SSH_KEY_VAULT_NAME" || -n "$SSH_KEY_VAULT_SECRET_NAME" ]]; then
    source=keyvault
  elif [[ -n "$SSH_PUBLIC_KEY_FILE" ]]; then
    source=local
  fi

  if [[ -z "$source" ]]; then
    selected_index=$(select_index "Existing SSH public key source" 0 \
      "SSH public key stored in Azure" \
      "Azure Key Vault secret containing an OpenSSH public key" \
      "Local OpenSSH public key file")
    case "$selected_index" in
      0) source=azure ;;
      1) source=keyvault ;;
      2) source=local ;;
    esac
  fi

  case "$source" in
    azure|azure-resource|resource|sshkey|ssh-key) choose_azure_ssh_public_key ;;
    keyvault|key-vault|vault) choose_key_vault_ssh_public_key ;;
    local|file) choose_local_ssh_public_key ;;
    *) fail "SSH_PUBLIC_KEY_SOURCE must be azure, keyvault, or local." ;;
  esac
}

choose_generated_ssh_key() {
  local default_private_key private_key

  default_private_key=${SSH_PRIVATE_KEY_FILE:-$HOME/.ssh/${VM_NAME}_ed25519}
  while :; do
    private_key=$(prompt_value "New SSH private key file" "$default_private_key")
    case "$private_key" in
      ~/*) private_key="$HOME/${private_key#\~/}" ;;
    esac
    if [[ "$private_key" != /* ]]; then
      printf 'SSH private key path must be absolute.\n' >&2
      continue
    fi
    private_key=$(realpath -m -- "$private_key")
    if [[ -e "$private_key" || -e "$private_key.pub" ]]; then
      printf 'Refusing to overwrite an existing SSH key: %s\n' "$private_key" >&2
      default_private_key=$private_key
      continue
    fi
    SSH_PRIVATE_KEY_FILE=$private_key
    SSH_PUBLIC_KEY_FILE="$private_key.pub"
    SSH_PUBLIC_KEY_DESCRIPTION="new Ed25519 key pair $private_key"
    return
  done
}

choose_vm_authentication() {
  local requested=${VM_AUTHENTICATION_TYPE,,} selected_index source=${SSH_PUBLIC_KEY_SOURCE,,}

  if [[ "$requested" == password ]] && \
    [[ -n "$source" || -n "$SSH_PRIVATE_KEY_FILE" || -n "$SSH_PUBLIC_KEY_FILE" || -n "$SSH_PUBLIC_KEY_RESOURCE_NAME" || \
      -n "$SSH_PUBLIC_KEY_RESOURCE_GROUP" || -n "$SSH_KEY_VAULT_NAME" || -n "$SSH_KEY_VAULT_SECRET_NAME" ]]; then
    fail "Password authentication cannot be combined with SSH public key settings."
  fi
  if [[ -z "$requested" ]]; then
    case "$source" in
      generate|generated|new|new-key) requested=generate ;;
      "")
        if [[ -n "$SSH_PUBLIC_KEY_FILE" || -n "$SSH_PUBLIC_KEY_RESOURCE_NAME" || \
          -n "$SSH_PUBLIC_KEY_RESOURCE_GROUP" || -n "$SSH_KEY_VAULT_NAME" || \
          -n "$SSH_KEY_VAULT_SECRET_NAME" ]]; then
          requested=ssh
        fi
        ;;
      *) requested=ssh ;;
    esac
  fi
  if [[ -z "$requested" ]]; then
    selected_index=$(select_index "VM administrator authentication" 0 \
      "Use an existing SSH public key (recommended)" \
      "Generate a new local Ed25519 SSH key pair" \
      "Use password authentication (less secure)")
    case "$selected_index" in
      0) requested=ssh ;;
      1) requested=generate ;;
      2) requested=password ;;
    esac
  fi

  case "$requested" in
    ssh|key|existing)
      require_command ssh-keygen
      VM_AUTHENTICATION_TYPE=ssh
      choose_ssh_public_key
      VM_AUTHENTICATION_DESCRIPTION="SSH public key: $SSH_PUBLIC_KEY_DESCRIPTION"
      ;;
    generate|generated|new|new-key)
      require_command ssh-keygen
      VM_AUTHENTICATION_TYPE=ssh
      GENERATE_SSH_KEY=true
      choose_generated_ssh_key
      VM_AUTHENTICATION_DESCRIPTION="SSH public key: $SSH_PUBLIC_KEY_DESCRIPTION"
      ;;
    password)
      VM_AUTHENTICATION_TYPE=password
      VM_AUTHENTICATION_DESCRIPTION="password (entered securely when Azure CLI creates the VM)"
      ;;
    *) fail "VM_AUTHENTICATION_TYPE must be ssh, generate, or password." ;;
  esac
}

prepare_vm_authentication() {
  local key_dir

  if [[ "$VM_AUTHENTICATION_TYPE" == password ]]; then
    VM_AUTHENTICATION_ARGS=(--authentication-type password)
    return
  fi
  if [[ "$GENERATE_SSH_KEY" == true ]]; then
    [[ ! -e "$SSH_PRIVATE_KEY_FILE" && ! -e "$SSH_PUBLIC_KEY_FILE" ]] || fail \
      "Refusing to overwrite an existing SSH key: $SSH_PRIVATE_KEY_FILE"
    key_dir=$(dirname -- "$SSH_PRIVATE_KEY_FILE")
    mkdir -p -- "$key_dir"
    [[ "$key_dir" != "$HOME/.ssh" ]] || chmod 0700 "$key_dir"
    printf '\nGenerating Ed25519 SSH key pair at %s.\n' "$SSH_PRIVATE_KEY_FILE" >&2
    printf 'ssh-keygen will prompt for an optional passphrase.\n' >&2
    ssh-keygen -t ed25519 -a 100 -f "$SSH_PRIVATE_KEY_FILE" -C "$ADMIN_USERNAME@$VM_NAME"
    SSH_PRIVATE_KEY_FILE=$(realpath -e -- "$SSH_PRIVATE_KEY_FILE")
    validate_ssh_public_key_file "$SSH_PRIVATE_KEY_FILE.pub"
  fi
  VM_AUTHENTICATION_ARGS=(--authentication-type ssh)
  VM_AUTHENTICATION_ARGS+=(--ssh-key-values "$SSH_PUBLIC_KEY_FILE")
}

ipv4_to_int() {
  local ip=$1
  local first second third fourth
  IFS=. read -r first second third fourth <<<"$ip"
  printf '%s\n' "$(((10#$first << 24) + (10#$second << 16) + (10#$third << 8) + 10#$fourth))"
}

validate_ipv4_cidr() {
  local label=$1
  local value=$2
  local allow_wildcard=${3:-false}
  local ip prefix extra octet octet_value ip_value mask
  local octets=()

  if [[ "$allow_wildcard" == true && "$value" == "*" ]]; then
    return
  fi

  IFS=/ read -r ip prefix extra <<<"$value"
  [[ "$value" == */* && -n "$ip" && -n "$prefix" && -z "$extra" ]] || fail "$label must be an IPv4 CIDR, for example 203.0.113.10/32."
  if [[ ! "$prefix" =~ ^[0-9]+$ ]] || ((10#$prefix > 32)); then
    fail "$label has an invalid prefix length: $value"
  fi

  IFS=. read -r -a octets <<<"$ip"
  ((${#octets[@]} == 4)) || fail "$label must contain four IPv4 octets: $value"
  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^[0-9]+$ ]] || fail "$label contains a non-numeric IPv4 octet: $value"
    [[ "$octet" == "0" || "$octet" != 0* ]] || fail "$label must not contain IPv4 octets with leading zeroes: $value"
    octet_value=$((10#$octet))
    ((octet_value <= 255)) || fail "$label contains an IPv4 octet greater than 255: $value"
  done

  ip_value=$(ipv4_to_int "$ip")
  if ((10#$prefix == 0)); then
    mask=0
  else
    mask=$(((0xFFFFFFFF << (32 - 10#$prefix)) & 0xFFFFFFFF))
  fi
  (( (ip_value & mask) == ip_value )) || fail "$label must use the network address for its prefix: $value"
}

cidr_contains() {
  local parent=$1
  local child=$2
  local parent_ip=${parent%/*}
  local parent_prefix=${parent#*/}
  local child_ip=${child%/*}
  local child_prefix=${child#*/}
  local parent_value child_value mask

  ((10#$child_prefix >= 10#$parent_prefix)) || return 1
  parent_value=$(ipv4_to_int "$parent_ip")
  child_value=$(ipv4_to_int "$child_ip")
  if ((10#$parent_prefix == 0)); then
    mask=0
  else
    mask=$(((0xFFFFFFFF << (32 - 10#$parent_prefix)) & 0xFFFFFFFF))
  fi
  (( (child_value & mask) == parent_value ))
}

choose_subscription() {
  local current_subscription_id default_index=0 index name subscription_id
  local subscription_records subscription_labels=() subscription_ids=()

  current_subscription_id=$(az account show --query id -o tsv)
  mapfile -t subscription_records < <(az account list --query "[?state=='Enabled'].[name,id]" -o tsv)
  ((${#subscription_records[@]} > 0)) || fail "No enabled Azure subscription is available."

  for index in "${!subscription_records[@]}"; do
    IFS=$'\t' read -r name subscription_id <<<"${subscription_records[$index]}"
    subscription_labels+=("$name ($subscription_id)")
    subscription_ids+=("$subscription_id")
    [[ "$subscription_id" == "$current_subscription_id" ]] && default_index=$index
  done

  if ((${#subscription_ids[@]} == 1)); then
    printf '%s\n' "${subscription_ids[0]}"
    return
  fi

  index=$(select_index "Azure subscription" "$default_index" "${subscription_labels[@]}")
  printf '%s\n' "${subscription_ids[$index]}"
}

choose_location() {
  local search search_lower index name display_name selected_index
  local location_records matches=() labels=() names=()

  mapfile -t location_records < <(
    az account list-locations \
      --query "[?metadata.regionType=='Physical'].[name,displayName]" \
      -o tsv | sort -k2
  )
  ((${#location_records[@]} > 0)) || fail "Azure returned no physical regions."

  while :; do
    search=$(prompt_value "Azure region name or search text" "$DEFAULT_LOCATION")
    search_lower=${search,,}
    matches=()
    labels=()
    names=()

    for index in "${!location_records[@]}"; do
      IFS=$'\t' read -r name display_name <<<"${location_records[$index]}"
      if [[ "${name,,}" == "$search_lower" ]]; then
        printf '%s\n' "$name"
        return
      fi
      if [[ "${name,,}" == *"$search_lower"* || "${display_name,,}" == *"$search_lower"* ]]; then
        matches+=("${location_records[$index]}")
        labels+=("$display_name ($name)")
        names+=("$name")
      fi
    done

    if ((${#matches[@]} == 0)); then
      printf 'No Azure region matched "%s".\n' "$search" >&2
      continue
    fi
    if ((${#matches[@]} > 15)); then
      printf '%d regions matched; enter more specific search text.\n' "${#matches[@]}" >&2
      continue
    fi
    if ((${#matches[@]} == 1)); then
      printf '%s\n' "${names[0]}"
      return
    fi

    selected_index=$(select_index "Matching Azure regions" 0 "${labels[@]}")
    printf '%s\n' "${names[$selected_index]}"
    return
  done
}

choose_vnet() {
  local index name resource_group selected_index
  local vnet_records vnet_labels=() vnet_names=() vnet_resource_groups=()

  mapfile -t vnet_records < <(
    az network vnet list \
      --query "[?location=='$LOCATION'].[name,resourceGroup]" \
      -o tsv
  )

  for index in "${!vnet_records[@]}"; do
    IFS=$'\t' read -r name resource_group <<<"${vnet_records[$index]}"
    vnet_labels+=("$name (resource group: $resource_group)")
    vnet_names+=("$name")
    vnet_resource_groups+=("$resource_group")
  done
  vnet_labels+=("Create a new VNet")

  selected_index=$(select_index "Virtual network in $LOCATION" 0 "${vnet_labels[@]}")
  if ((selected_index == ${#vnet_names[@]})); then
    VNET_ACTION=create
    VNET_NAME=$(prompt_value "New VNet name" "${VM_NAME}-vnet")
    VNET_RESOURCE_GROUP=$RESOURCE_GROUP
    VNET_PREFIX=$(prompt_value "New VNet address prefix" "10.42.0.0/24")
    SUBNET_ACTION=create-with-vnet
    SUBNET_NAME=$(prompt_value "New subnet name" "${VM_NAME}-subnet")
    SUBNET_PREFIX=$(prompt_value "New subnet address prefix" "10.42.0.0/27")
    validate_resource_name "VNet name" "$VNET_NAME"
    validate_resource_name "Subnet name" "$SUBNET_NAME"
    validate_ipv4_cidr "VNet address prefix" "$VNET_PREFIX"
    validate_ipv4_cidr "Subnet address prefix" "$SUBNET_PREFIX"
    cidr_contains "$VNET_PREFIX" "$SUBNET_PREFIX" || fail "Subnet prefix $SUBNET_PREFIX is not contained in VNet prefix $VNET_PREFIX."
    return
  fi

  VNET_ACTION=existing
  VNET_NAME=${vnet_names[$selected_index]}
  VNET_RESOURCE_GROUP=${vnet_resource_groups[$selected_index]}
}

choose_subnet() {
  local index name resource_id selected_index
  local subnet_records subnet_labels=() subnet_names=() subnet_ids=()

  [[ "$VNET_ACTION" == "existing" ]] || return
  mapfile -t subnet_records < <(
    az network vnet subnet list \
      --resource-group "$VNET_RESOURCE_GROUP" \
      --vnet-name "$VNET_NAME" \
      --query "[?length(delegations)==\`0\`].[name,id]" \
      -o tsv
  )

  for index in "${!subnet_records[@]}"; do
    IFS=$'\t' read -r name resource_id <<<"${subnet_records[$index]}"
    subnet_labels+=("$name")
    subnet_names+=("$name")
    subnet_ids+=("$resource_id")
  done
  subnet_labels+=("Create a new subnet")

  selected_index=$(select_index "Non-delegated subnet in $VNET_NAME" 0 "${subnet_labels[@]}")
  if ((selected_index == ${#subnet_names[@]})); then
    SUBNET_ACTION=create
    SUBNET_NAME=$(prompt_value "New subnet name" "${VM_NAME}-subnet")
    SUBNET_PREFIX=$(prompt_value "New subnet address prefix")
    [[ -n "$SUBNET_PREFIX" ]] || fail "A subnet address prefix is required."
    validate_resource_name "Subnet name" "$SUBNET_NAME"
    validate_ipv4_cidr "Subnet address prefix" "$SUBNET_PREFIX"
    return
  fi

  SUBNET_ACTION=existing
  SUBNET_NAME=${subnet_names[$selected_index]}
  SUBNET_ID=${subnet_ids[$selected_index]}
}

choose_vm_size() {
  local available_skus candidate index selected_index custom_size
  local recommended_sizes=(Standard_B1ms Standard_B1s Standard_B2s Standard_B2ms Standard_D2s_v5)
  local recommended_descriptions=(
    "Standard_B1ms - 1 vCPU, 2 GiB (recommended for low traffic)"
    "Standard_B1s - 1 vCPU, 1 GiB (minimum for very low traffic)"
    "Standard_B2s - 2 vCPU, 4 GiB"
    "Standard_B2ms - 2 vCPU, 8 GiB"
    "Standard_D2s_v5 - 2 vCPU, 8 GiB"
  )
  local size_labels=() size_values=()

  printf '\nLoading VM sizes available in %s...\n' "$LOCATION" >&2
  available_skus=$(az vm list-skus \
    --location "$LOCATION" \
    --resource-type virtualMachines \
    --all \
    --query "[?length(restrictions[?type=='Location'])==\`0\`].name" \
    -o tsv)

  for index in "${!recommended_sizes[@]}"; do
    candidate=${recommended_sizes[$index]}
    if grep -Fxq "$candidate" <<<"$available_skus"; then
      size_labels+=("${recommended_descriptions[$index]}")
      size_values+=("$candidate")
    fi
  done
  size_labels+=("Enter another available VM size")

  selected_index=$(select_index "VM size" 0 "${size_labels[@]}")
  if ((selected_index < ${#size_values[@]})); then
    VM_SIZE=${size_values[$selected_index]}
    return
  fi

  while :; do
    custom_size=$(prompt_value "VM size, for example Standard_B1ms")
    if grep -Fxq "$custom_size" <<<"$available_skus"; then
      VM_SIZE=$custom_size
      return
    fi
    printf '%s is not available for this subscription in %s.\n' "$custom_size" "$LOCATION" >&2
  done
}

resolve_image_requirements() {
  local image_metadata

  image_metadata=$(az vm image show \
    --location "$LOCATION" \
    --urn "$UBUNTU_IMAGE" \
    --query '[architecture,hyperVGeneration]' \
    -o tsv) || fail "Azure could not resolve VM image $UBUNTU_IMAGE in $LOCATION."
  IFS=$'\t' read -r IMAGE_ARCHITECTURE IMAGE_HYPERV_GENERATION <<<"$image_metadata"
  [[ -n "$IMAGE_ARCHITECTURE" && -n "$IMAGE_HYPERV_GENERATION" ]] || \
    fail "Azure returned incomplete architecture metadata for $UBUNTU_IMAGE."
}

validate_vm_image_compatibility() {
  local vm_architecture vm_generations

  vm_architecture=$(az vm list-skus \
    --location "$LOCATION" \
    --resource-type virtualMachines \
    --size "$VM_SIZE" \
    --all \
    --query "[?name=='$VM_SIZE'] | [0].capabilities[?name=='CpuArchitectureType'] | [0].value" \
    -o tsv)
  vm_generations=$(az vm list-skus \
    --location "$LOCATION" \
    --resource-type virtualMachines \
    --size "$VM_SIZE" \
    --all \
    --query "[?name=='$VM_SIZE'] | [0].capabilities[?name=='HyperVGenerations'] | [0].value" \
    -o tsv)

  [[ "$vm_architecture" == "$IMAGE_ARCHITECTURE" ]] || fail \
    "VM size $VM_SIZE uses $vm_architecture, but image $UBUNTU_IMAGE requires $IMAGE_ARCHITECTURE."
  case ",$vm_generations," in
    *",$IMAGE_HYPERV_GENERATION,"*) ;;
    *) fail "VM size $VM_SIZE supports Hyper-V $vm_generations, but image $UBUNTU_IMAGE requires $IMAGE_HYPERV_GENERATION." ;;
  esac
}

choose_disk() {
  local selected_index custom_size premium_io
  local disk_labels=("StandardSSD_LRS - balanced cost and latency (recommended)")
  local disk_values=(StandardSSD_LRS)
  local size_labels=(
    "32 GiB / E4 - minimum tier for the default Ubuntu image (recommended)"
    "64 GiB / E6"
    "128 GiB / E10"
    "Enter another size"
  )
  local size_values=(32 64 128)

  premium_io=$(az vm list-skus \
    --location "$LOCATION" \
    --resource-type virtualMachines \
    --size "$VM_SIZE" \
    --all \
    --query "[?name=='$VM_SIZE'] | [0].capabilities[?name=='PremiumIO'] | [0].value" \
    -o tsv)
  if [[ "${premium_io,,}" == "true" ]]; then
    disk_labels+=("Premium_LRS - higher IOPS")
    disk_values+=(Premium_LRS)
  fi
  disk_labels+=("Standard_LRS - lowest cost HDD")
  disk_values+=(Standard_LRS)

  selected_index=$(select_index "OS disk type" 0 "${disk_labels[@]}")
  DISK_SKU=${disk_values[$selected_index]}

  selected_index=$(select_index "OS disk size" 0 "${size_labels[@]}")
  if ((selected_index < ${#size_values[@]})); then
    DISK_SIZE_GB=${size_values[$selected_index]}
    return
  fi

  while :; do
    custom_size=$(prompt_value "OS disk size in GiB (30-4095)")
    if [[ "$custom_size" =~ ^[0-9]+$ ]] && ((custom_size >= 30 && custom_size <= 4095)); then
      DISK_SIZE_GB=$custom_size
      return
    fi
    printf 'Enter an integer between 30 and 4095.\n' >&2
  done
}

detect_public_ip() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- -T 5 https://api.ipify.org 2>/dev/null || true
  fi
}

run_as_root() {
  if ((EUID == 0)); then
    "$@"
    return
  fi
  command -v sudo >/dev/null 2>&1 || fail "Administrator privileges are required. Install sudo or run the command as root."
  sudo "$@"
}

start_local_docker_service() {
  if command -v systemctl >/dev/null 2>&1 && run_as_root systemctl enable --now docker; then
    return
  fi
  if command -v service >/dev/null 2>&1 && run_as_root service docker start; then
    return
  fi
  fail "Docker is installed, but its service could not be started automatically."
}

install_local_docker() {
  command -v apt-get >/dev/null 2>&1 || fail \
    "Docker is not installed. Automatic installation is supported on Debian/Ubuntu hosts with apt-get; install Docker Engine and rerun this script."

  printf 'Docker was not found. This host can install the distribution docker.io package.\n' >&2
  confirm "Install Docker Engine and enable its service now" || fail \
    "Docker is required for local container deployment."
  run_as_root apt-get update
  run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io
  hash -r
  command -v docker >/dev/null 2>&1 || fail "Docker installation completed, but the docker command is still unavailable."
  start_local_docker_service
}

continue_with_docker_group() {
  local target_user quoted_script

  [[ "${DOCKER_GROUP_RETRY:-false}" != true ]] || fail \
    "Docker is still inaccessible after applying docker group membership. Sign out and back in, then rerun this script."
  target_user=${SUDO_USER:-${USER:-}}
  [[ -n "$target_user" ]] || target_user=$(id -un)
  [[ "$target_user" != root ]] || fail "Docker is running, but root cannot access its socket. Check the Docker daemon logs."

  if [[ " $(id -nG "$target_user") " != *" docker "* ]]; then
    printf 'The docker group grants root-equivalent access to this host.\n' >&2
    confirm "Add $target_user to the docker group" || fail \
      "The current user cannot access Docker. Grant Docker socket access and rerun this script."
    run_as_root usermod -aG docker "$target_user"
  fi

  command -v sg >/dev/null 2>&1 || fail \
    "Docker access was updated. Sign out and back in, then rerun this script."
  printf 'Restarting local deployment with the updated docker group membership...\n' >&2
  printf -v quoted_script '%q' "$0"
  if sg docker -c "DEPLOY_MODE=local DOCKER_GROUP_RETRY=true exec bash $quoted_script"; then
    exit 0
  fi
  fail "The temporary docker group session failed. Sign out and back in, then rerun this script."
}

ensure_local_docker() {
  local current_context docker_error service_stopped=false

  if ! command -v docker >/dev/null 2>&1; then
    install_local_docker
  fi
  if docker_error=$(docker info --format '{{.ServerVersion}}' 2>&1); then
    return
  fi

  current_context=$(docker context show 2>/dev/null || printf 'default\n')
  if [[ -z "${DOCKER_HOST:-}" && "$current_context" == default ]]; then
    if command -v systemctl >/dev/null 2>&1; then
      systemctl is-active --quiet docker || service_stopped=true
    elif command -v service >/dev/null 2>&1; then
      service docker status >/dev/null 2>&1 || service_stopped=true
    fi
  fi
  if [[ "$service_stopped" == true ]]; then
    printf 'Docker is installed, but its system service is not running.\n' >&2
    confirm "Start and enable Docker now" || fail "A running Docker Engine is required."
    start_local_docker_service
    if docker_error=$(docker info --format '{{.ServerVersion}}' 2>&1); then
      return
    fi
  fi

  if [[ -z "${DOCKER_HOST:-}" && -S /var/run/docker.sock && "${docker_error,,}" == *"permission denied"* ]]; then
    continue_with_docker_group
  fi

  docker_error=${docker_error//$'\n'/'; '}
  fail "Docker is installed but not reachable${docker_error:+: $docker_error}"
}

find_checkout_launcher() {
  local script_dir checkout_root

  script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
  checkout_root=$(cd -- "$script_dir/../.." && pwd)
  if [[ -x "$checkout_root/start.sh" && -f "$checkout_root/.env.example" && -f "$checkout_root/config/platform.example.json" ]]; then
    LOCAL_INSTALL_DIR=$checkout_root
    LOCAL_LAUNCHER="$checkout_root/start.sh"
  elif [[ -x "$LOCAL_INSTALL_DIR/start.sh" && -f "$LOCAL_INSTALL_DIR/config/platform.example.json" ]] && \
    [[ -f "$LOCAL_INSTALL_DIR/.env" || -f "$LOCAL_INSTALL_DIR/.env.example" ]]; then
    LOCAL_LAUNCHER="$LOCAL_INSTALL_DIR/start.sh"
  else
    LOCAL_LAUNCHER=
  fi
}

download_support_file() {
  local relative_path=$1 temp_file

  temp_file=$(mktemp)
  TEMP_FILES+=("$temp_file")
  printf 'Downloading %s...\n' "$relative_path" >&2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 10 --max-time 60 "$DEPLOY_SCRIPT_BASE_URL/$relative_path" -o "$temp_file"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$temp_file" --timeout=60 --tries=2 "$DEPLOY_SCRIPT_BASE_URL/$relative_path"
  else
    fail "curl or wget is required to install the local container launcher."
  fi
  [[ -s "$temp_file" ]] || fail "Downloaded file is empty: $relative_path"
  DOWNLOADED_FILE=$temp_file
}

prepare_local_launcher() {
  local parent_dir staging_dir start_file env_file config_file

  [[ -n "$LOCAL_LAUNCHER" ]] && return
  [[ ! -e "$LOCAL_INSTALL_DIR" ]] || fail \
    "Launcher directory already exists but is incomplete: $LOCAL_INSTALL_DIR. Use a complete checkout or choose another LOCAL_INSTALL_DIR."

  require_command install
  download_support_file start.sh
  start_file=$DOWNLOADED_FILE
  download_support_file .env.example
  env_file=$DOWNLOADED_FILE
  download_support_file config/platform.example.json
  config_file=$DOWNLOADED_FILE
  sh -n "$start_file" || fail "Downloaded start.sh failed its syntax check."

  parent_dir=$(dirname -- "$LOCAL_INSTALL_DIR")
  mkdir -p -- "$parent_dir"
  staging_dir=$(mktemp -d "$parent_dir/.caddy-reverse-proxy.XXXXXX")
  TEMP_DIRS+=("$staging_dir")
  install -d -m 0755 "$staging_dir/config"
  install -m 0755 "$start_file" "$staging_dir/start.sh"
  install -m 0644 "$env_file" "$staging_dir/.env.example"
  install -m 0644 "$config_file" "$staging_dir/config/platform.example.json"
  [[ ! -e "$LOCAL_INSTALL_DIR" ]] || fail "Launcher directory was created concurrently: $LOCAL_INSTALL_DIR"
  mv -T -- "$staging_dir" "$LOCAL_INSTALL_DIR"
  LOCAL_LAUNCHER="$LOCAL_INSTALL_DIR/start.sh"
}

validate_local_networks() {
  local network
  local networks=()

  [[ -z "$LOCAL_DOCKER_NETWORKS" ]] && return
  [[ "$LOCAL_DOCKER_NETWORKS" != *[[:space:]]* ]] || fail "Docker network list must not contain whitespace."
  IFS=, read -r -a networks <<<"$LOCAL_DOCKER_NETWORKS"
  for network in "${networks[@]}"; do
    [[ -n "$network" ]] || fail "Docker network list contains an empty name."
    docker network inspect "$network" >/dev/null 2>&1 || fail "Docker network does not exist: $network"
  done
}

validate_local_port() {
  local label=$1 value=$2

  if [[ ! "$value" =~ ^[0-9]+$ ]] || ((10#$value < 1 || 10#$value > 65535)); then
    fail "$label must be an integer between 1 and 65535."
  fi
}

validate_local_settings() {
  local data_root

  [[ "$LOCAL_CONTAINER_NAME" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || \
    fail "CONTAINER_NAME must start with a letter or digit and contain only letters, digits, periods, underscores, or hyphens."
  validate_local_port HTTP_PORT "$LOCAL_HTTP_PORT"
  validate_local_port HTTPS_PORT "$LOCAL_HTTPS_PORT"
  validate_local_port MANAGEMENT_PORT "$LOCAL_MANAGEMENT_PORT"
  [[ "$LOCAL_HTTP_PORT" != "$LOCAL_HTTPS_PORT" && \
    "$LOCAL_HTTP_PORT" != "$LOCAL_MANAGEMENT_PORT" && \
    "$LOCAL_HTTPS_PORT" != "$LOCAL_MANAGEMENT_PORT" ]] || \
    fail "HTTP_PORT, HTTPS_PORT, and MANAGEMENT_PORT must be different."

  data_root=$(realpath -m -- "$HOME/docker_files")
  LOCAL_DATA_DIR=$(realpath -m -- "$LOCAL_DATA_DIR")
  case "$LOCAL_DATA_DIR" in
    "$data_root"/*) ;;
    *) fail "DATA_DIR must resolve below $data_root." ;;
  esac
}

deploy_local_container() {
  local networks_display

  ensure_local_docker
  require_command realpath
  LOCAL_INSTALL_DIR=$(realpath -m -- "$LOCAL_INSTALL_DIR")
  [[ "$LOCAL_INSTALL_DIR" != / ]] || fail "LOCAL_INSTALL_DIR cannot be the filesystem root."
  find_checkout_launcher
  if [[ -z "$LOCAL_LAUNCHER" ]]; then
    LOCAL_INSTALL_DIR=$(prompt_value "Launcher install directory" "$LOCAL_INSTALL_DIR")
    [[ "$LOCAL_INSTALL_DIR" == /* ]] || fail "Launcher install directory must be an absolute path."
    LOCAL_INSTALL_DIR=$(realpath -m -- "$LOCAL_INSTALL_DIR")
    [[ "$LOCAL_INSTALL_DIR" != / ]] || fail "Launcher install directory cannot be the filesystem root."
    find_checkout_launcher
  fi
  [[ "$LOCAL_DATA_DIR" == /* ]] || fail "DATA_DIR must be an absolute path."
  validate_local_settings
  LOCAL_DOCKER_NETWORKS=$(prompt_value "Existing Docker networks, comma-separated (optional)" "$LOCAL_DOCKER_NETWORKS")
  validate_local_networks
  networks_display=${LOCAL_DOCKER_NETWORKS:-none}

  printf '\nLocal container deployment summary\n' >&2
  printf '  Image:         %s\n' "$IMAGE" >&2
  printf '  Launcher:      %s\n' "$LOCAL_INSTALL_DIR" >&2
  printf '  Container:     %s\n' "$LOCAL_CONTAINER_NAME" >&2
  printf '  Persistent data: %s\n' "$LOCAL_DATA_DIR" >&2
  printf '  Ports:         %s->80, %s->443, 127.0.0.1:%s->8080\n' \
    "$LOCAL_HTTP_PORT" "$LOCAL_HTTPS_PORT" "$LOCAL_MANAGEMENT_PORT" >&2
  printf '  Docker networks: %s\n' "$networks_display" >&2
  printf '  Azure/DNS:     no infrastructure or DNS changes\n' >&2

  confirm "Deploy the gateway container on this machine" || fail "Deployment cancelled."
  prepare_local_launcher

  env \
    IMAGE="$IMAGE" \
    CONTAINER_NAME="$LOCAL_CONTAINER_NAME" \
    DATA_DIR="$LOCAL_DATA_DIR" \
    HTTP_PORT="$LOCAL_HTTP_PORT" \
    HTTPS_PORT="$LOCAL_HTTPS_PORT" \
    MANAGEMENT_PORT="$LOCAL_MANAGEMENT_PORT" \
    DOCKER_NETWORKS="$LOCAL_DOCKER_NETWORKS" \
    "$LOCAL_LAUNCHER" start

  printf '\nContainer lifecycle commands:\n' >&2
  printf '  CONTAINER_NAME=%q DATA_DIR=%q %q stop\n' \
    "$LOCAL_CONTAINER_NAME" "$LOCAL_DATA_DIR" "$LOCAL_LAUNCHER" >&2
  printf '  IMAGE=%q CONTAINER_NAME=%q DATA_DIR=%q HTTP_PORT=%q HTTPS_PORT=%q MANAGEMENT_PORT=%q DOCKER_NETWORKS=%q %q start\n' \
    "$IMAGE" "$LOCAL_CONTAINER_NAME" "$LOCAL_DATA_DIR" "$LOCAL_HTTP_PORT" "$LOCAL_HTTPS_PORT" \
    "$LOCAL_MANAGEMENT_PORT" "$LOCAL_DOCKER_NETWORKS" "$LOCAL_LAUNCHER" >&2
}

write_cloud_init() {
  local cloud_init_file=$1

  cat >"$cloud_init_file" <<CLOUD_INIT
#cloud-config
package_update: true
packages:
  - ca-certificates
  - curl
  - docker.io
  - openssl
write_files:
  - path: /etc/sysctl.d/60-caddy-reverse-proxy.conf
    permissions: '0644'
    content: |
      net.ipv4.ip_unprivileged_port_start=80
runcmd:
  - [systemctl, enable, --now, docker]
  - |
      set -eu
      install -d -m 0700 /etc/caddy-reverse-proxy
      install -d -m 0700 /var/lib/caddy-reverse-proxy/platform
      token=\$(openssl rand -hex 32)
      printf '%s\n' "\$token" >/var/lib/caddy-reverse-proxy/platform/admin-token
      chmod 0600 /var/lib/caddy-reverse-proxy/platform/admin-token
      printf '%s\n' \
        'GATEWAY_PROFILE=vm' \
        'GATEWAY_DEPLOYMENT_MODE=azure-vm' \
        'GATEWAY_CONTROL_LISTEN=127.0.0.1:8080' \
        "GATEWAY_ADMIN_TOKEN=\$token" \
        'GATEWAY_AUTH_REQUIRED=true' \
        'GATEWAY_DOCKER_ENABLED=false' \
        'GATEWAY_AZURE_ENABLED=false' \
        >/etc/caddy-reverse-proxy/gateway.env
      chmod 0600 /etc/caddy-reverse-proxy/gateway.env
      docker pull '$IMAGE'
      docker run -d \
        --name caddy-reverse-proxy \
        --restart unless-stopped \
        --env-file /etc/caddy-reverse-proxy/gateway.env \
        --network host \
        -v /var/lib/caddy-reverse-proxy:/data \
        --security-opt no-new-privileges:true \
        --health-cmd='wget -q -T 2 -O /dev/null http://127.0.0.1:8080/readyz || exit 1' \
        --health-interval=15s \
        --health-timeout=3s \
        --health-retries=4 \
        --health-start-period=30s \
        '$IMAGE'
CLOUD_INIT
}

wait_for_gateway() {
  local output remote_script

  read -r -d '' remote_script <<'REMOTE_SCRIPT' || true
set -eu
cloud-init status --wait
attempt=0
while [ "$attempt" -lt 60 ]; do
  if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
    health=$(docker inspect caddy-reverse-proxy --format '{{.State.Health.Status}}' 2>/dev/null || true)
    if [ "$health" = healthy ]; then
      printf "GATEWAY_ADMIN_TOKEN=%s\n" "$(cat /var/lib/caddy-reverse-proxy/platform/admin-token)"
      docker inspect caddy-reverse-proxy --format "CONTAINER={{.State.Status}} HEALTH={{.State.Health.Status}}"
      exit 0
    fi
  fi
  attempt=$((attempt + 1))
  sleep 5
done
docker inspect caddy-reverse-proxy --format "CONTAINER={{.State.Status}} HEALTH={{if .State.Health}}{{.State.Health.Status}}{{else}}unavailable{{end}}" >&2 || true
docker logs --tail 100 caddy-reverse-proxy >&2 || true
exit 1
REMOTE_SCRIPT

  printf '\nWaiting for cloud-init, Docker, and gateway readiness...\n' >&2
  output=$(az vm run-command invoke \
    --resource-group "$RESOURCE_GROUP" \
    --name "$VM_NAME" \
    --command-id RunShellScript \
    --scripts "$remote_script" \
    --query 'value[0].message' \
    -o tsv)

  ADMIN_TOKEN=$(sed -n 's/^GATEWAY_ADMIN_TOKEN=//p' <<<"$output" | tail -n 1)
  if [[ -z "$ADMIN_TOKEN" ]]; then
    printf '%s\n' "$output" >&2
    fail "The gateway did not become healthy, or its admin token could not be parsed from Azure Run Command output."
  fi
}

[[ "$IMAGE" =~ ^[A-Za-z0-9._/@:-]+$ ]] || fail "IMAGE contains unsupported characters."
[[ "$ROLLBACK_ON_ERROR" == true || "$ROLLBACK_ON_ERROR" == false ]] || fail "ROLLBACK_ON_ERROR must be true or false."

SELECTED_DEPLOY_MODE=$(choose_deployment_mode)
if [[ "$SELECTED_DEPLOY_MODE" == local ]]; then
  printf 'Caddy Reverse Proxy - local container deployment\n' >&2
  deploy_local_container
  exit 0
fi

require_command az
require_command find
require_command grep
require_command realpath
require_command sed

if ! az account show >/dev/null 2>&1; then
  printf 'No active Azure CLI login was found. Starting device-code login...\n' >&2
  az login --use-device-code >/dev/null
fi

printf 'Caddy Reverse Proxy - standalone Azure VM deployment\n' >&2
printf 'This script creates Azure resources but does not configure DNS or application routes.\n' >&2

SUBSCRIPTION_ID=$(choose_subscription)
az account set --subscription "$SUBSCRIPTION_ID"
SUBSCRIPTION_NAME=$(az account show --query name -o tsv)

LOCATION=$(choose_location)
resolve_image_requirements
VM_NAME=$(prompt_value "VM name" "$DEFAULT_VM_NAME")
RESOURCE_GROUP=$(prompt_value "Deployment resource group" "$DEFAULT_RESOURCE_GROUP")
ADMIN_USERNAME=$(prompt_value "Linux admin username" "$DEFAULT_ADMIN_USERNAME")
validate_resource_name "VM name" "$VM_NAME"
validate_admin_username "$ADMIN_USERNAME"

choose_vnet
choose_subnet
choose_vm_size
validate_vm_image_compatibility
choose_disk
choose_vm_authentication

DETECTED_IP=$(detect_public_ip)
DEFAULT_SSH_SOURCE=${SSH_SOURCE_CIDR:-}
if [[ -z "$DEFAULT_SSH_SOURCE" && "$DETECTED_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  DEFAULT_SSH_SOURCE="$DETECTED_IP/32"
fi
while :; do
  SSH_SOURCE_CIDR=$(prompt_value "SSH source CIDR (use * only when a fixed source is unavailable)" "$DEFAULT_SSH_SOURCE")
  [[ -n "$SSH_SOURCE_CIDR" ]] && break
  printf 'An SSH source CIDR is required for the management tunnel.\n' >&2
done
validate_ipv4_cidr "SSH source CIDR" "$SSH_SOURCE_CIDR" true

NSG_NAME="${VM_NAME}-nsg"
PUBLIC_IP_NAME="${VM_NAME}-pip"
NIC_NAME="${VM_NAME}-nic"
OS_DISK_NAME="${VM_NAME}-osdisk"

RESOURCE_GROUP_EXISTS=$(az group exists --name "$RESOURCE_GROUP" -o tsv)
if [[ "$RESOURCE_GROUP_EXISTS" == true ]]; then
  if az vm show --resource-group "$RESOURCE_GROUP" --name "$VM_NAME" >/dev/null 2>&1; then
    fail "VM already exists: $VM_NAME"
  fi
  if az disk show --resource-group "$RESOURCE_GROUP" --name "$OS_DISK_NAME" >/dev/null 2>&1; then
    fail "OS disk already exists: $OS_DISK_NAME"
  fi
  for resource_check in \
    "nsg:$NSG_NAME" \
    "public-ip:$PUBLIC_IP_NAME" \
    "nic:$NIC_NAME"; do
    resource_type=${resource_check%%:*}
    resource_name=${resource_check#*:}
    if az network "$resource_type" show --resource-group "$RESOURCE_GROUP" --name "$resource_name" >/dev/null 2>&1; then
      fail "Resource already exists: $resource_name"
    fi
  done
  if [[ "$VNET_ACTION" == "create" ]] && az network vnet show \
    --resource-group "$RESOURCE_GROUP" \
    --name "$VNET_NAME" >/dev/null 2>&1; then
    fail "VNet already exists: $VNET_NAME"
  fi
fi

if [[ "$SUBNET_ACTION" == "create" ]] && az network vnet subnet show \
  --resource-group "$VNET_RESOURCE_GROUP" \
  --vnet-name "$VNET_NAME" \
  --name "$SUBNET_NAME" >/dev/null 2>&1; then
  fail "Subnet already exists: $SUBNET_NAME"
fi

printf '\nDeployment summary\n' >&2
printf '  Subscription:  %s (%s)\n' "$SUBSCRIPTION_NAME" "$SUBSCRIPTION_ID" >&2
printf '  Region:        %s\n' "$LOCATION" >&2
printf '  Resource group:%s\n' " $RESOURCE_GROUP" >&2
printf '  VM:            %s (%s, Ubuntu 24.04)\n' "$VM_NAME" "$VM_SIZE" >&2
printf '  OS image:      %s (%s, Hyper-V %s)\n' \
  "$UBUNTU_IMAGE" "$IMAGE_ARCHITECTURE" "$IMAGE_HYPERV_GENERATION" >&2
printf '  VNet/subnet:   %s / %s\n' "$VNET_NAME" "$SUBNET_NAME" >&2
printf '  OS disk:       %s GiB %s\n' "$DISK_SIZE_GB" "$DISK_SKU" >&2
printf '  Authentication:%s\n' " $VM_AUTHENTICATION_DESCRIPTION" >&2
printf '  Public ports:  TCP 80, 443; TCP 22 from %s\n' "$SSH_SOURCE_CIDR" >&2
printf '  Container image: %s\n' "$IMAGE" >&2
printf '  Rollback:      remove new VM/network resources; retain the resource group\n' >&2
printf '  DNS/routes:    not configured by this script\n' >&2

confirm "Create these Azure resources" || fail "Deployment cancelled."

prepare_vm_authentication
DEPLOYMENT_STARTED=true
if [[ "$RESOURCE_GROUP_EXISTS" == true ]]; then
  printf '\nUsing existing resource group %s.\n' "$RESOURCE_GROUP" >&2
else
  printf '\nCreating resource group...\n' >&2
  az group create --name "$RESOURCE_GROUP" --location "$LOCATION" --output none
fi

if [[ "$VNET_ACTION" == "create" ]]; then
  printf 'Creating VNet and subnet...\n' >&2
  CREATED_VNET=true
  az network vnet create \
    --resource-group "$RESOURCE_GROUP" \
    --location "$LOCATION" \
    --name "$VNET_NAME" \
    --tags application=caddy-reverse-proxy deployment=vm \
    --address-prefixes "$VNET_PREFIX" \
    --subnet-name "$SUBNET_NAME" \
    --subnet-prefixes "$SUBNET_PREFIX" \
    --output none
  SUBNET_ID=$(az network vnet subnet show \
    --resource-group "$RESOURCE_GROUP" \
    --vnet-name "$VNET_NAME" \
    --name "$SUBNET_NAME" \
    --query id -o tsv)
elif [[ "$SUBNET_ACTION" == "create" ]]; then
  printf 'Creating subnet in existing VNet...\n' >&2
  CREATED_SUBNET=true
  az network vnet subnet create \
    --resource-group "$VNET_RESOURCE_GROUP" \
    --vnet-name "$VNET_NAME" \
    --name "$SUBNET_NAME" \
    --address-prefixes "$SUBNET_PREFIX" \
    --output none
  SUBNET_ID=$(az network vnet subnet show \
    --resource-group "$VNET_RESOURCE_GROUP" \
    --vnet-name "$VNET_NAME" \
    --name "$SUBNET_NAME" \
    --query id -o tsv)
fi

printf 'Creating NSG, static public IP, and NIC...\n' >&2
MANAGED_NSG=true
az network nsg create \
  --resource-group "$RESOURCE_GROUP" \
  --location "$LOCATION" \
  --name "$NSG_NAME" \
  --tags application=caddy-reverse-proxy deployment=vm \
  --output none
NSG_ID=$(az network nsg show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$NSG_NAME" \
  --query id -o tsv)

az network nsg rule create \
  --resource-group "$RESOURCE_GROUP" \
  --nsg-name "$NSG_NAME" \
  --name AllowHttpHttps \
  --priority 100 \
  --direction Inbound \
  --access Allow \
  --protocol Tcp \
  --source-address-prefixes Internet \
  --destination-port-ranges 80 443 \
  --output none

az network nsg rule create \
  --resource-group "$RESOURCE_GROUP" \
  --nsg-name "$NSG_NAME" \
  --name AllowSshFromOperator \
  --priority 110 \
  --direction Inbound \
  --access Allow \
  --protocol Tcp \
  --source-address-prefixes "$SSH_SOURCE_CIDR" \
  --destination-port-ranges 22 \
  --output none

MANAGED_PUBLIC_IP=true
az network public-ip create \
  --resource-group "$RESOURCE_GROUP" \
  --location "$LOCATION" \
  --name "$PUBLIC_IP_NAME" \
  --sku Standard \
  --allocation-method Static \
  --version IPv4 \
  --tags application=caddy-reverse-proxy deployment=vm \
  --output none
PUBLIC_IP_ID=$(az network public-ip show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$PUBLIC_IP_NAME" \
  --query id -o tsv)

MANAGED_NIC=true
az network nic create \
  --resource-group "$RESOURCE_GROUP" \
  --location "$LOCATION" \
  --name "$NIC_NAME" \
  --subnet "$SUBNET_ID" \
  --network-security-group "$NSG_ID" \
  --public-ip-address "$PUBLIC_IP_ID" \
  --tags application=caddy-reverse-proxy deployment=vm \
  --output none
NIC_ID=$(az network nic show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$NIC_NAME" \
  --query id -o tsv)

CLOUD_INIT_FILE=$(mktemp)
TEMP_FILES+=("$CLOUD_INIT_FILE")
write_cloud_init "$CLOUD_INIT_FILE"

printf 'Creating VM and installing the gateway through cloud-init...\n' >&2
VM_CREATE_ATTEMPTED=true
az vm create \
  --resource-group "$RESOURCE_GROUP" \
  --location "$LOCATION" \
  --name "$VM_NAME" \
  --nics "$NIC_ID" \
  --image "$UBUNTU_IMAGE" \
  --size "$VM_SIZE" \
  --admin-username "$ADMIN_USERNAME" \
  "${VM_AUTHENTICATION_ARGS[@]}" \
  --assign-identity \
  --os-disk-name "$OS_DISK_NAME" \
  --os-disk-size-gb "$DISK_SIZE_GB" \
  --storage-sku "$DISK_SKU" \
  --os-disk-delete-option Delete \
  --custom-data "$CLOUD_INIT_FILE" \
  --tags application=caddy-reverse-proxy deployment=vm \
  --output none

wait_for_gateway

PUBLIC_IP=$(az network public-ip show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$PUBLIC_IP_NAME" \
  --query ipAddress -o tsv)
PRIVATE_IP=$(az network nic show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$NIC_NAME" \
  --query 'ipConfigurations[0].privateIPAddress' -o tsv)
PRINCIPAL_ID=$(az vm identity show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$VM_NAME" \
  --query principalId -o tsv)

DEPLOYMENT_SUCCEEDED=true

SSH_IDENTITY_OPTION=
if [[ "$GENERATE_SSH_KEY" == true ]]; then
  printf -v SSH_IDENTITY_OPTION ' -i %q' "$SSH_PRIVATE_KEY_FILE"
fi

cat <<EOF

Deployment completed.

VM:                    $VM_NAME
Resource group:        $RESOURCE_GROUP
Region:                $LOCATION
Public IP:             $PUBLIC_IP
Private IP:            $PRIVATE_IP
Managed identity:      $PRINCIPAL_ID
Admin token:           $ADMIN_TOKEN

SSH:
  ssh$SSH_IDENTITY_OPTION $ADMIN_USERNAME@$PUBLIC_IP

Management UI tunnel:
  ssh$SSH_IDENTITY_OPTION -L 8080:127.0.0.1:8080 $ADMIN_USERNAME@$PUBLIC_IP
  Open http://127.0.0.1:8080

Next manual steps:
  1. Point each DNS A record to $PUBLIC_IP.
  2. Open the management UI through the SSH tunnel and configure explicit routes.
  3. Allow traffic from $PRIVATE_IP (or its subnet) in each private backend NSG/firewall.
  4. If using Azure DNS-01, grant this VM identity DNS Zone Contributor on the DNS zone.

The gateway data is stored on the VM under /var/lib/caddy-reverse-proxy.
It is stored on the OS disk; deleting the VM also deletes this state unless it is backed up first.
DNS records and application routes were not changed.
EOF
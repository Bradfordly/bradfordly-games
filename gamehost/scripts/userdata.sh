#!/bin/bash
set -e

# ============================================================
# gamehost userdata script for start up
# ============================================================

# --- configuration variables ---
# all parameters sourced from aws ssm parameter store

AWS_REGION=$(curl -sL http://169.254.169.254/latest/meta-data/placement/region)
SNS_TOPIC_ARN=$(aws ssm get-parameter --name "/snsTopicArn" --with-decryption --query "Parameter.Value" --output text --region $AWS_REGION)

# ============================================================
# Installation Functions
# ============================================================

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a /var/log/pterodactyl-userdata.log
}

send_sns_notification() {
    local subject="$1"
    local message="$2"
    
    # Get instance metadata
    TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
    INSTANCE_ID=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/instance-id)
    PUBLIC_IP=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/public-ipv4 || echo "N/A")
    PRIVATE_IP=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/local-ipv4)
    
    # Build notification message
    full_message="Pterodactyl Installation Status
================================
Instance ID: $INSTANCE_ID
Public IP: $PUBLIC_IP
Private IP: $PRIVATE_IP
Timestamp: $(date -u '+%Y-%m-%d %H:%M:%S UTC')

$message

Panel URL: http://$PUBLIC_IP
Admin Username: $user_username
Admin Email: $user_email"

    # Send SNS notification
    aws sns publish \
        --region "$AWS_REGION" \
        --topic-arn "$SNS_TOPIC_ARN" \
        --subject "$subject" \
        --message "$full_message" || log "WARNING: Failed to send SNS notification"
}

# ============================================================
# Main Installation
# ============================================================

log "Starting Pterodactyl Panel installation..."

# Update FQDN to use public IP if set to localhost
if [ "$FQDN" == "localhost" ]; then
    TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
    PUBLIC_IP=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/public-ipv4 || echo "")
    if [ -n "$PUBLIC_IP" ]; then
        export FQDN="$PUBLIC_IP"
        log "FQDN set to public IP: $FQDN"
    fi
fi

# Install AWS CLI if not present (for SNS notifications)
if ! command -v aws &> /dev/null; then
    log "Installing AWS CLI..."
    if command -v apt-get &> /dev/null; then
        apt-get update -y
        apt-get install -y awscli
    elif command -v dnf &> /dev/null; then
        dnf install -y awscli
    elif command -v yum &> /dev/null; then
        yum install -y awscli
    fi
fi

# Send start notification
send_sns_notification "Pterodactyl Installation Started" "Installation process has begun on the EC2 instance."

# Run the Pterodactyl installer
log "Downloading and running Pterodactyl installer..."

# Download the installer script
curl -sSL -o /tmp/pterodactyl-install.sh https://pterodactyl-installer.se

# Source the lib.sh to get functions
export GITHUB_SOURCE="v1.2.0"
export SCRIPT_RELEASE="v1.2.0"
export GITHUB_BASE_URL="https://raw.githubusercontent.com/pterodactyl-installer/pterodactyl-installer"

# Download and source lib.sh
curl -sSL -o /tmp/lib.sh "$GITHUB_BASE_URL/master/lib/lib.sh"
source /tmp/lib.sh

# Run the panel installer directly (non-interactive)
log "Running panel installer..."
if bash <(curl -sSL "$GITHUB_BASE_URL/$GITHUB_SOURCE/installers/panel.sh"); then
    INSTALL_STATUS="SUCCESS"
    INSTALL_MESSAGE="Pterodactyl Panel has been successfully installed!

Access Details:
- Panel URL: http://$FQDN
- Admin Username: $user_username
- Admin Email: $user_email
- Database: $MYSQL_DB
- Database User: $MYSQL_USER

Next Steps:
1. Configure DNS to point to this server
2. Set up SSL/TLS certificates for production use
3. Configure Wings on game server nodes"
    
    log "Installation completed successfully!"
else
    INSTALL_STATUS="FAILED"
    INSTALL_MESSAGE="Pterodactyl Panel installation failed. Please check the logs at:
- /var/log/pterodactyl-installer.log
- /var/log/pterodactyl-userdata.log"
    
    log "Installation failed!"
fi

# Send completion notification
send_sns_notification "Pterodactyl Installation $INSTALL_STATUS" "$INSTALL_MESSAGE"

log "Userdata script completed with status: $INSTALL_STATUS"

# Cleanup
rm -f /tmp/lib.sh /tmp/pterodactyl-install.sh

exit 0

# PACKAGES defines a list of packages to install when running make install
# PACKAGES := \
# 	git \
# 	curl

# install:
# 	make install-packages
# 	make install-rust
# 	make install-aws-cli
# 	make install-cleanup

# .PHONY: install-packages
# install-packages:
# 	sudo apt-get update
# 	sudo apt-get install -y $(PACKAGES)

# .PHONY: install-rust
# install-rust:
# 	curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
# 	source $(HOME)/.cargo/env

# .PHONY: install-aws-cli
# install-aws-cli:
# 	curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
# 	unzip awscliv2.zip
# 	sudo ./aws/install
# 	aws configure set region us-east-1
# 	aws configure set output json
# 	aws configure set aws_access_key_id $(ACCESS_KEY)
# 	aws configure set aws_secret_access_key $(SECRET_KEY)

# .PHONY: install-cleanup
# install-cleanup:
# 	rm -rf awscliv2.zip
# 	rm -rf aws
# WIP -- intended use case is to setup a new github codespace
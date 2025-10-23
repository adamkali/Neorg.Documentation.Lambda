# create an environment varibale to use for authentication testing
AUTH_TOKEN=test-token-123
CONTAINER_NAME=neorg.documentation.lambda
HUMAN_READABLE_NAME=Neorg.Documentation.Lambda
USERNAME=adamkali
DOCKER_REGISTRY=ghcr.io
PORT=2025

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.DEFAULT_GOAL := install

clean-test-data:
	@echo "$(YELLOW)Cleaning up test data:$(NC)"
	rm -f *.tar *.zip
	@echo "$(GREEN)Test data cleaned$(NC)"

docker-build: clean-test-data 
	@echo "$(BLUE)Building $(CONTAINER_NAME):$(NC)"
	docker build -t $(CONTAINER_NAME):$(shell git rev-parse --short HEAD) .

docker-run:
	@echo "$(BLUE)  Running $(CONTAINER_NAME):$(NC)"
	docker run --platform linux/amd64 -d --name $(HUMAN_READABLE_NAME) -p ${PORT}:${PORT} -e PORT=${PORT} -e NEORG_DOCUMENTATION_AUTH_TOKEN=$(AUTH_TOKEN) --restart unless-stopped $(CONTAINER_NAME):$(shell git rev-parse --short HEAD)

docker-stop:
	@echo "$(RED)Stopping $(CONTAINER_NAME):$(NC)"
	docker stop $(HUMAN_READABLE_NAME)

docker-remove:
	@echo "$(RED)Removing $(CONTAINER_NAME):$(NC)"
	docker rm $(HUMAN_READABLE_NAME) 

docker-logs:
	docker logs $(HUMAN_READABLE_NAME) --tail 10

documentation:
	nvim --headless -c "cd ./docgen" -c "source simple_norg_converter.lua" -c 'qa'

test-data:
	# create uncompressed tarball of docs directory 
	tar -cvf test.tar docs
	curl -X POST -H "Content-Type: application/x-tar" -H "x-auth-token: $(AUTH_TOKEN)" --data-binary @test.tar http://localhost:${PORT} --output converted_docs.zip

test-unzip:
	@echo "$(BLUE)Extracting converted documentation:$(NC)"
	@if [ -f converted_docs.zip ]; then \
		unzip -o converted_docs.zip && \
		echo "$(GREEN)Extracted files:$(NC)" && \
		ls -la *.md 2>/dev/null || echo "$(YELLOW)No markdown files found$(NC)"; \
	else \
		echo "$(RED)No converted_docs.zip found. Run 'make test-data' first.$(NC)"; \
	fi

push-to-registry:
	@echo "$(BLUE)Pushing $(CONTAINER_NAME):$(NC)"
	@echo "$(YELLOW)  $(DOCKER_REGISTRY)/$(CONTAINER_NAME):$(NC) will be uploaded."
	docker tag $(CONTAINER_NAME):$(shell git rev-parse --short HEAD) $(DOCKER_REGISTRY)/$(USERNAME)/$(CONTAINER_NAME):$(shell git rev-parse --short HEAD)
	docker push $(DOCKER_REGISTRY)/$(USERNAME)/$(CONTAINER_NAME):$(shell git rev-parse --short HEAD)
	@echo "$(GREEN)  Pushed $(DOCKER_REGISTRY)/$(CONTAINER_NAME):$(NC)"

install: clean-test-data docker-build

run: clean-test-data docker-build docker-run test-data test-unzip 

upload: clean-test-data docker-build push-to-registry


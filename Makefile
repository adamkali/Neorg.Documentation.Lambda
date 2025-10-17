AUTH_TOKEN=test-token-123

zip:
	zip -r docs.zip test
	curl -H "x-auth-token: $(AUTH_TOKEN)" http://localhost:8080/health
	curl -X POST -H "x-auth-token: $(AUTH_TOKEN)" -H "Content-Type: application/zip" --data-binary @test.zip -o __test.zip http://localhost:8080/

docker-build: 
	docker build -t neorg-lambda .

docker-run:
	docker run --platform linux/amd64 -d --name neorg-converter -p 8080:8080 -e NEORG_DOCUMENTATION_AUTH_TOKEN=$(AUTH_TOKEN) --restart unless-stopped neorg-lambda

docker-stop:
	docker stop neorg-converter

docker-remove:
	docker rm neorg-converter

docker-logs:
	docker logs neorg-converter --tail 10



.PHONY: install-docker docker-post-setup docker-build docker-run docker-redeploy

install-docker:
	sudo apt update
	sudo apt install -y ca-certificates curl gnupg
	sudo install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
	echo "deb [arch=$$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $$(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
	sudo apt update
	sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

docker-post-setup:
	sudo usermod -aG docker $${USER}
	@echo "Relogin required for docker group membership to take effect."

# Сборка Docker-образа (тег по умолчанию — последняя короткая SHA)
docker-build:
	docker build -t showsparser:$$(git rev-parse --short HEAD) .

# Запуск контейнера с актуальным образом
docker-run:
	docker run -d --rm --name showsparser \
		--env-file /opt/showsparser/.env \
		-v /opt/showsparser/.env:/app/.env:ro \
		showsparser:$$(git rev-parse --short HEAD)

# Полный цикл на сервере
docker-redeploy:
	git pull
	docker build -t showsparser:$$(git rev-parse --short HEAD) .
	@if docker ps -aq --filter name=showsparser | grep -q .; then \
		docker stop showsparser && docker rm showsparser; \
	fi
	docker run -d --name showsparser \
		--env-file /opt/showsparser/.env \
		-v /opt/showsparser/.env:/app/.env:ro \
		showsparser:$$(git rev-parse --short HEAD)
FROM python:3.12-slim

WORKDIR /app
COPY pyproject.toml ./pyproject.toml
COPY src ./src
RUN pip install --no-cache-dir .

ENTRYPOINT ["python", "-m", "request_logger_http.app"]

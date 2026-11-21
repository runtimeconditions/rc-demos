package io.runtimeconditions.demos.requestlogger;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class RequestLoggerHttp {
    private RequestLoggerHttp() {
    }

    public static void main(String[] args) throws IOException {
        int port = Integer.parseInt(envOrDefault("PORT", "8080"));
        ExecutorService workers = Executors.newCachedThreadPool();
        try (ServerSocket server = new ServerSocket(port)) {
            System.out.println("request-logger Java demo listening on :" + port);
            while (true) {
                Socket socket = server.accept();
                workers.submit(() -> handleConnection(socket));
            }
        }
    }

    private static void handleConnection(Socket socket) {
        try (socket) {
            socket.setSoTimeout(5_000);
            BufferedReader reader = new BufferedReader(
                    new InputStreamReader(socket.getInputStream(), StandardCharsets.UTF_8));
            String requestLine = reader.readLine();
            if (requestLine == null || requestLine.isBlank()) {
                return;
            }
            String header;
            while ((header = reader.readLine()) != null && !header.isEmpty()) {
                // Drain request headers; the demo endpoints only route on the path.
            }

            String[] parts = requestLine.split(" ");
            String path = parts.length > 1 ? parts[1] : "/";
            int query = path.indexOf('?');
            if (query >= 0) {
                path = path.substring(0, query);
            }

            OutputStream out = socket.getOutputStream();
            switch (path) {
                case "/ready" -> readinessHandler(out);
                case "/demo" -> demoHandler(out);
                default -> respond(out, 404, "Not Found", "text/plain", "not found\n".getBytes(StandardCharsets.UTF_8));
            }
        } catch (IOException e) {
            System.err.println("request handling failed: " + e.getMessage());
        }
    }

    private static void readinessHandler(OutputStream out) throws IOException {
        String todos = statusString(checkTodosApi());
        String cache = statusString(checkRedis());
        if (!"ok".equals(todos) || !"ok".equals(cache)) {
            writeJson(out, 503, "Service Unavailable",
                    "One or more dependencies are not healthy: todosApi=" + todos + ", cache=" + cache);
            return;
        }
        respond(out, 204, "No Content", null, new byte[0]);
    }

    private static void demoHandler(OutputStream out) throws IOException {
        String todos = statusString(checkTodosApi());
        String cache = statusString(checkRedis());
        boolean healthy = "ok".equals(todos) && "ok".equals(cache);
        writeJson(out, healthy ? 200 : 500, healthy ? "OK" : "Internal Server Error",
                "{\"todosApi\":\"" + todos + "\",\"cache\":\"" + cache + "\"}\n");
    }

    private static Exception checkTodosApi() {
        String baseUrl = trimTrailingSlash(System.getenv("TODOS_API_URL"));
        if (baseUrl.isBlank()) {
            return new IllegalStateException("TODOS_API_URL is not set");
        }
        try {
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(baseUrl + "/todos/1"))
                    .timeout(Duration.ofSeconds(3))
                    .GET()
                    .build();
            HttpResponse<String> response = HttpClient.newHttpClient()
                    .send(request, HttpResponse.BodyHandlers.ofString());
            if (response.statusCode() != 200) {
                return new IllegalStateException("todos-api returned HTTP " + response.statusCode());
            }
            String body = response.body();
            if (!body.contains("\"id\"") || !body.contains("\"title\"")) {
                return new IllegalStateException("todos-api response was incomplete");
            }
            return null;
        } catch (Exception e) {
            return e;
        }
    }

    private static Exception checkRedis() {
        try {
            InetSocketAddress address = redisAddress();
            try (Socket socket = new Socket()) {
                socket.connect(address, 3_000);
                socket.setSoTimeout(3_000);
                socket.getOutputStream().write("*1\r\n$4\r\nPING\r\n".getBytes(StandardCharsets.UTF_8));
                byte[] response = socket.getInputStream().readNBytes(7);
                String text = new String(response, StandardCharsets.UTF_8);
                if (!text.startsWith("+PONG")) {
                    return new IllegalStateException("redis ping returned " + text.strip());
                }
            }
            return null;
        } catch (Exception e) {
            return e;
        }
    }

    private static InetSocketAddress redisAddress() {
        String rawUrl = System.getenv("REDIS_URL");
        if (rawUrl != null && !rawUrl.isBlank()) {
            URI uri = URI.create(rawUrl);
            if (uri.getHost() != null && uri.getPort() > 0) {
                return new InetSocketAddress(uri.getHost(), uri.getPort());
            }
        }

        String host = System.getenv("REDIS_HOST");
        if (host == null || host.isBlank()) {
            throw new IllegalStateException("REDIS_URL or REDIS_HOST must be set");
        }
        int port = Integer.parseInt(envOrDefault("REDIS_PORT", "6379"));
        return new InetSocketAddress(host, port);
    }

    private static String statusString(Exception error) {
        if (error == null) {
            return "ok";
        }
        System.err.println(error.getMessage());
        return "error";
    }

    private static void writeJson(OutputStream out, int status, String reason, String body) throws IOException {
        respond(out, status, reason, "application/json", body.getBytes(StandardCharsets.UTF_8));
    }

    private static void respond(OutputStream out, int status, String reason, String contentType, byte[] body)
            throws IOException {
        StringBuilder headers = new StringBuilder();
        headers.append("HTTP/1.1 ").append(status).append(' ').append(reason).append("\r\n");
        if (contentType != null) {
            headers.append("Content-Type: ").append(contentType).append("\r\n");
        }
        headers.append("Content-Length: ").append(body.length).append("\r\n");
        headers.append("Connection: close\r\n\r\n");
        out.write(headers.toString().getBytes(StandardCharsets.US_ASCII));
        if (body.length > 0) {
            out.write(body);
        }
        out.flush();
    }

    private static String trimTrailingSlash(String value) {
        if (value == null) {
            return "";
        }
        while (value.endsWith("/")) {
            value = value.substring(0, value.length() - 1);
        }
        return value;
    }

    private static String envOrDefault(String key, String fallback) {
        String value = System.getenv(key);
        return value == null || value.isBlank() ? fallback : value;
    }
}

package com.alephcom.teams.cucmjtapi;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Objects;
import java.util.logging.Level;
import java.util.logging.Logger;

/** POSTs line-state JSON to the Go sync service. */
public final class LineStatePoster {
  private static final Logger LOG = Logger.getLogger(LineStatePoster.class.getName());

  private final HttpClient client;
  private final URI endpoint;
  private final String token;

  public LineStatePoster(String endpointUrl, String token) {
    this.client = HttpClient.newBuilder()
        .connectTimeout(Duration.ofSeconds(5))
        .build();
    this.endpoint = URI.create(endpointUrl);
    this.token = token == null ? "" : token;
  }

  public void post(String extension, String state) {
    Objects.requireNonNull(extension, "extension");
    Objects.requireNonNull(state, "state");
    String json = "{\"extension\":\"" + escape(extension) + "\",\"state\":\"" + escape(state) + "\"}";
    HttpRequest.Builder b = HttpRequest.newBuilder(endpoint)
        .timeout(Duration.ofSeconds(10))
        .header("Content-Type", "application/json")
        .POST(HttpRequest.BodyPublishers.ofString(json, StandardCharsets.UTF_8));
    if (!token.isEmpty()) {
      b.header("X-CUCM-Token", token);
    }
    try {
      HttpResponse<String> resp = client.send(b.build(), HttpResponse.BodyHandlers.ofString());
      int code = resp.statusCode();
      if (code < 200 || code >= 300) {
        LOG.warning("POST line-state failed: HTTP " + code + " for " + extension + "=" + state);
      } else {
        LOG.info("posted " + extension + " -> " + state);
      }
    } catch (IOException | InterruptedException e) {
      LOG.log(Level.WARNING, "POST line-state error for " + extension, e);
      if (e instanceof InterruptedException) {
        Thread.currentThread().interrupt();
      }
    }
  }

  private static String escape(String s) {
    return s.replace("\\", "\\\\").replace("\"", "\\\"");
  }
}

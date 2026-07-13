package com.alephcom.teams.cucmjtapi;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashSet;
import java.util.Locale;
import java.util.Set;

/** Environment / file configuration for the sidecar. */
public final class Config {
  public final String[] hosts;
  public final String username;
  public final String password;
  public final Set<String> extensions;
  public final String eventUrl;
  public final String eventToken;
  public final long reconnectDelayMs;

  private Config(String[] hosts, String username, String password, Set<String> extensions,
                 String eventUrl, String eventToken, long reconnectDelayMs) {
    this.hosts = hosts;
    this.username = username;
    this.password = password;
    this.extensions = extensions;
    this.eventUrl = eventUrl;
    this.eventToken = eventToken;
    this.reconnectDelayMs = reconnectDelayMs;
  }

  public static Config fromEnv() throws IOException {
    String hostsRaw = env("CUCM_HOSTS", env("CUCM_HOST", "127.0.0.1"));
    String[] hosts = Arrays.stream(hostsRaw.split(","))
        .map(String::trim)
        .filter(s -> !s.isEmpty())
        .toArray(String[]::new);
    if (hosts.length == 0) {
      throw new IllegalArgumentException("CUCM_HOSTS is required");
    }

    String username = env("CUCM_USERNAME", "");
    String password = env("CUCM_PASSWORD", "");
    if (username.isEmpty() || password.isEmpty()) {
      throw new IllegalArgumentException("CUCM_USERNAME and CUCM_PASSWORD are required");
    }

    Set<String> extensions = loadExtensions(
        env("CUCM_EXTENSIONS", ""),
        env("CUCM_EXTENSIONS_FILE", ""));

    String eventUrl = env("CUCM_EVENT_URL", "http://127.0.0.1:8090/v1/line-state");
    String eventToken = env("CUCM_EVENT_TOKEN", "");
    long delay = Long.parseLong(env("CUCM_RECONNECT_MS", "5000"));

    return new Config(hosts, username, password, extensions, eventUrl, eventToken, delay);
  }

  private static Set<String> loadExtensions(String csv, String filePath) throws IOException {
    Set<String> set = new LinkedHashSet<>();
    if (csv != null && !csv.isBlank()) {
      for (String p : csv.split(",")) {
        String e = p.trim();
        if (!e.isEmpty()) {
          set.add(e);
        }
      }
    }
    if (filePath != null && !filePath.isBlank()) {
      Path path = Path.of(filePath);
      if (!Files.isRegularFile(path)) {
        throw new IOException("CUCM_EXTENSIONS_FILE not found: " + filePath);
      }
      String text = Files.readString(path, StandardCharsets.UTF_8);
      // Support plain one-DN-per-line, or reuse extensions.json-ish "extension":"NNN"
      if (filePath.toLowerCase(Locale.ROOT).endsWith(".json")) {
        // Minimal extract of "extension" values without a JSON library
        int i = 0;
        while (true) {
          int key = text.indexOf("\"extension\"", i);
          if (key < 0) {
            break;
          }
          int colon = text.indexOf(':', key);
          int q1 = text.indexOf('"', colon + 1);
          int q2 = text.indexOf('"', q1 + 1);
          if (colon < 0 || q1 < 0 || q2 < 0) {
            break;
          }
          String ext = text.substring(q1 + 1, q2).trim();
          if (!ext.isEmpty()) {
            set.add(ext);
          }
          i = q2 + 1;
        }
      } else {
        for (String line : text.split("\n")) {
          line = line.trim();
          if (line.isEmpty() || line.startsWith("#")) {
            continue;
          }
          // CSV first column
          int comma = line.indexOf(',');
          if (comma > 0) {
            line = line.substring(0, comma).trim();
          }
          if ("extension".equalsIgnoreCase(line)) {
            continue;
          }
          set.add(line);
        }
      }
    }
    return set.isEmpty() ? Collections.emptySet() : set;
  }

  private static String env(String key, String def) {
    String v = System.getenv(key);
    return v == null || v.isBlank() ? def : v.trim();
  }

  @Override
  public String toString() {
    return "Config{hosts=" + Arrays.toString(hosts)
        + ", user=" + username
        + ", extensions=" + extensions.size()
        + ", eventUrl=" + eventUrl
        + ", reconnectMs=" + reconnectDelayMs + "}";
  }
}

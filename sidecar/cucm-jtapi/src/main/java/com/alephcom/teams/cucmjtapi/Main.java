package com.alephcom.teams.cucmjtapi;

import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * CUCM JTAPI sidecar: observe line state and POST to the Go sync service.
 *
 * <pre>
 *   PROVIDER=cucm on the Go side (listens on CUCM_EVENT_LISTEN)
 *   This process connects to CUCM CTI Manager (TCP 2748) via JTAPI.
 * </pre>
 */
public final class Main {
  private static final Logger LOG = Logger.getLogger(Main.class.getName());

  private Main() {}

  public static void main(String[] args) throws Exception {
    Config cfg = Config.fromEnv();
    LOG.info("starting cucm-jtapi sidecar: " + cfg);

    LineStatePoster poster = new LineStatePoster(cfg.eventUrl, cfg.eventToken);
    Runtime.getRuntime().addShutdownHook(new Thread(() -> LOG.info("shutting down")));

    int hostIdx = 0;
    while (true) {
      String host = cfg.hosts[hostIdx % cfg.hosts.length];
      hostIdx++;
      JtapiObserver observer = new JtapiObserver(
          host, cfg.username, cfg.password, cfg.extensions, poster);
      try {
        observer.runUntilDisconnected();
      } catch (Exception e) {
        LOG.log(Level.SEVERE, "JTAPI session ended on " + host, e);
      } finally {
        observer.stop();
      }
      LOG.info("reconnecting in " + cfg.reconnectDelayMs + "ms (next host in list)");
      Thread.sleep(cfg.reconnectDelayMs);
    }
  }
}

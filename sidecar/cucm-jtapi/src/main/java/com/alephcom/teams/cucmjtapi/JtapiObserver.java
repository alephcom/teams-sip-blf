package com.alephcom.teams.cucmjtapi;

import com.cisco.jtapi.extensions.CiscoProvider;
import com.cisco.jtapi.extensions.CiscoTerminalConnection;

import javax.telephony.Address;
import javax.telephony.Call;
import javax.telephony.CallObserver;
import javax.telephony.Connection;
import javax.telephony.Provider;
import javax.telephony.ProviderObserver;
import javax.telephony.Terminal;
import javax.telephony.TerminalConnection;
import javax.telephony.events.CallEv;
import javax.telephony.events.ProvEv;
import javax.telephony.events.ProvInServiceEv;
import javax.telephony.events.ProvOutOfServiceEv;
import javax.telephony.events.ProvShutdownEv;

import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Opens a JTAPI provider, observes configured DNs, and posts state changes.
 */
public final class JtapiObserver implements ProviderObserver, CallObserver {
  private static final Logger LOG = Logger.getLogger(JtapiObserver.class.getName());

  private final String host;
  private final String username;
  private final String password;
  private final Set<String> watchDns;
  private final LineStatePoster poster;
  private final Map<String, String> lastState = new HashMap<>();
  private final AtomicBoolean running = new AtomicBoolean(true);
  private final CountDownLatch outOfService = new CountDownLatch(1);

  private Provider provider;

  public JtapiObserver(String host, String username, String password,
                       Set<String> watchDns, LineStatePoster poster) {
    this.host = host;
    this.username = username;
    this.password = password;
    this.watchDns = watchDns;
    this.poster = poster;
  }

  /** Connect and block until provider goes out of service or {@link #stop()} is called. */
  public void runUntilDisconnected() throws Exception {
    String providerString = host + ";login=" + username + ";passwd=" + password;
    LOG.info("connecting JTAPI provider to " + host);
    javax.telephony.JtapiPeer peer = javax.telephony.JtapiPeerFactory.getJtapiPeer(null);
    provider = peer.getProvider(providerString);
    provider.addObserver(this);

    Thread.sleep(2000);
    openAddresses();
    snapshotAll();

    while (running.get()) {
      if (outOfService.await(5, TimeUnit.SECONDS)) {
        LOG.warning("provider out of service on " + host);
        break;
      }
    }
  }

  public void stop() {
    running.set(false);
    outOfService.countDown();
    closeQuietly();
  }

  private void openAddresses() throws Exception {
    if (provider instanceof CiscoProvider) {
      LOG.fine("using CiscoProvider");
    }
    Address[] addresses = provider.getAddresses();
    if (addresses == null) {
      LOG.warning("no addresses on provider (check Controlled Devices for app user)");
      return;
    }
    int opened = 0;
    for (Address addr : addresses) {
      String dn = normalizeDn(addr.getName());
      if (!watchDns.isEmpty() && !watchDns.contains(dn)) {
        continue;
      }
      addr.addCallObserver(this);
      opened++;
      LOG.info("observing DN " + dn);
    }
    if (opened == 0) {
      LOG.warning("no matching addresses opened; watch list=" + watchDns
          + " — ensure Controlled Devices include those phones");
    }
  }

  private void snapshotAll() {
    try {
      Address[] addresses = provider.getAddresses();
      if (addresses == null) {
        return;
      }
      for (Address addr : addresses) {
        String dn = normalizeDn(addr.getName());
        if (!watchDns.isEmpty() && !watchDns.contains(dn)) {
          continue;
        }
        emit(dn, stateForAddress(addr));
      }
    } catch (Exception e) {
      LOG.log(Level.WARNING, "snapshot failed", e);
    }
  }

  @Override
  public void providerChangedEvent(ProvEv[] events) {
    if (events == null) {
      return;
    }
    for (ProvEv ev : events) {
      if (ev instanceof ProvInServiceEv) {
        LOG.info("provider in service");
        try {
          openAddresses();
          snapshotAll();
        } catch (Exception e) {
          LOG.log(Level.WARNING, "openAddresses after in-service", e);
        }
      } else if (ev instanceof ProvOutOfServiceEv || ev instanceof ProvShutdownEv) {
        LOG.warning("provider event: " + ev.getClass().getSimpleName());
        outOfService.countDown();
      }
    }
  }

  @Override
  public void callChangedEvent(CallEv[] events) {
    if (events == null || events.length == 0) {
      return;
    }
    Set<String> touched = new HashSet<>();
    for (CallEv ev : events) {
      try {
        Call call = ev.getCall();
        if (call == null) {
          continue;
        }
        Connection[] conns = call.getConnections();
        if (conns == null) {
          continue;
        }
        for (Connection c : conns) {
          Address a = c.getAddress();
          if (a != null) {
            String dn = normalizeDn(a.getName());
            if (watchDns.isEmpty() || watchDns.contains(dn)) {
              touched.add(dn);
            }
          }
        }
      } catch (Exception e) {
        LOG.log(Level.FINE, "call event parse", e);
      }
    }
    for (String dn : touched) {
      try {
        Address addr = findAddress(dn);
        if (addr != null) {
          emit(dn, stateForAddress(addr));
        }
      } catch (Exception e) {
        LOG.log(Level.WARNING, "state update for " + dn, e);
      }
    }
  }

  private Address findAddress(String dn) throws Exception {
    Address[] addresses = provider.getAddresses();
    if (addresses == null) {
      return null;
    }
    for (Address a : addresses) {
      if (dn.equals(normalizeDn(a.getName()))) {
        return a;
      }
    }
    return null;
  }

  private String stateForAddress(Address addr) {
    boolean ringing = false;
    boolean talking = false;
    boolean held = false;
    try {
      Connection[] conns = addr.getConnections();
      if (conns != null) {
        for (Connection c : conns) {
          int cs = c.getState();
          if (cs == Connection.ALERTING) {
            ringing = true;
          } else if (cs == Connection.CONNECTED) {
            talking = true;
          }
          TerminalConnection[] tcs = c.getTerminalConnections();
          if (tcs != null) {
            for (TerminalConnection tc : tcs) {
              int ts = tc.getState();
              if (ts == TerminalConnection.RINGING) {
                ringing = true;
              } else if (ts == TerminalConnection.TALKING) {
                talking = true;
              } else if (ts == TerminalConnection.HELD) {
                held = true;
              }
              if (tc instanceof CiscoTerminalConnection) {
                String mapped = StateMapper.fromTerminalConnectionState(String.valueOf(ts));
                if (StateMapper.RINGING.equals(mapped)) {
                  ringing = true;
                } else if (StateMapper.BUSY.equals(mapped)) {
                  talking = true;
                }
              }
            }
          }
        }
      }
      Terminal[] terms = addr.getTerminals();
      if (terms != null) {
        for (Terminal t : terms) {
          if (t == null) {
            continue;
          }
          TerminalConnection[] tcs = t.getTerminalConnections();
          if (tcs == null) {
            continue;
          }
          for (TerminalConnection tc : tcs) {
            int ts = tc.getState();
            if (ts == TerminalConnection.RINGING) {
              ringing = true;
            } else if (ts == TerminalConnection.TALKING) {
              talking = true;
            } else if (ts == TerminalConnection.HELD) {
              held = true;
            }
          }
        }
      }
    } catch (Exception e) {
      LOG.log(Level.FINE, "stateForAddress " + addr.getName(), e);
    }
    return StateMapper.fromFlags(ringing, talking, held);
  }

  private void emit(String dn, String state) {
    String prev = lastState.put(dn, state);
    if (state.equals(prev)) {
      return;
    }
    poster.post(dn, state);
  }

  static String normalizeDn(String name) {
    if (name == null) {
      return "";
    }
    String n = name.trim();
    int sep = n.indexOf(' ');
    if (sep > 0) {
      n = n.substring(0, sep);
    }
    return n;
  }

  private void closeQuietly() {
    if (provider == null) {
      return;
    }
    try {
      provider.shutdown();
    } catch (Exception e) {
      LOG.log(Level.FINE, "provider shutdown", e);
    }
    provider = null;
  }
}

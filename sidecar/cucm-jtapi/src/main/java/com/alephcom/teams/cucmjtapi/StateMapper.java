package com.alephcom.teams.cucmjtapi;

import java.util.Locale;

/**
 * Maps JTAPI terminal-connection / call conditions to Go blf.State values:
 * idle | ringing | busy (HOLD maps to busy).
 */
public final class StateMapper {
  private StateMapper() {}

  public static final String IDLE = "idle";
  public static final String RINGING = "ringing";
  public static final String BUSY = "busy";

  /**
   * Aggregate per-address state from observed call legs.
   *
   * @param hasRinging true if any connection is alerting/ringing
   * @param hasTalking true if any connection is talking/active
   * @param hasHeld true if any connection is held
   */
  public static String fromFlags(boolean hasRinging, boolean hasTalking, boolean hasHeld) {
    if (hasRinging) {
      return RINGING;
    }
    if (hasTalking || hasHeld) {
      return BUSY;
    }
    return IDLE;
  }

  /** Normalize a JTAPI TerminalConnection state name if available. */
  public static String fromTerminalConnectionState(String jtapiStateName) {
    if (jtapiStateName == null) {
      return IDLE;
    }
    String s = jtapiStateName.toUpperCase(Locale.ROOT);
    if (s.contains("RING") || s.contains("ALERT")) {
      return RINGING;
    }
    if (s.contains("HOLD")) {
      return BUSY;
    }
    if (s.contains("TALK") || s.contains("ACTIVE") || s.contains("BRIDGED")) {
      return BUSY;
    }
    return IDLE;
  }
}

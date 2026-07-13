# Cisco JTAPI jars (not committed)

Download **Cisco JTAPI Client for Linux** from CUCM 15:

Admin → **Application → Plugins** → Cisco JTAPI Client for Linux → `CiscoJTAPILinux.zip`

Unpack and copy `jtapi.jar` (and any other jars from the zip that JTAPI needs) into this directory as:

```
lib/jtapi.jar
```

The sidecar will not compile or run without these proprietary jars.

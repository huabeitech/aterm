#!/bin/sh
binary='__ATERM_BINARY__'
root='__ATERM_DIRECTORY__/connections'
# Only an interactive destination without a remote command is integrated.
# Unsupported modes and explicit multiplexing settings keep their original semantics.
eligible=1
need=0
target=0
for arg do
  if [ "$need" = 1 ]; then need=0; continue; fi
  if [ "$target" = 1 ]; then eligible=0; break; fi
  case "$arg" in
    --) eligible=0; break ;;
    -*)
      flags=${arg#-}
      while [ -n "$flags" ]; do
        rest=${flags#?}
        flag=${flags%"$rest"}
        flags=$rest
        case "$flag" in
          f|G|N|M|O|Q|V|s|W|n|S) eligible=0; break ;;
          b|B|c|D|E|e|F|I|i|J|L|l|m|o|p|R|w)
            [ -z "$flags" ] && need=1
            break ;;
          4|6|A|a|C|g|K|k|q|T|t|v|X|x|Y|y) ;;
          *) eligible=0; break ;;
        esac
      done ;;
    *) target=1 ;;
  esac
done
# Also detect separate '-o Control...' arguments without interpreting shell text.
for arg do
  case "$arg" in ControlMaster*|ControlPath*|ControlPersist*|RemoteCommand*|SessionType*|RequestTTY*) eligible=0 ;; esac
done
if [ "$eligible" != 1 ] || [ "$target" != 1 ]; then exec "$binary" "$@"; fi
connection=$(mktemp -d "$root/c.XXXXXXXX") || exec "$binary" "$@"
"$binary" -G "$@" > "$connection/config.tmp" 2>/dev/null || { rm -rf "$connection"; exec "$binary" "$@"; }
# Configured command-only sessions must not acquire a remote file workspace.
if grep -Eq '^(remotecommand .+|sessiontype (none|subsystem)|requesttty no|controlpath .+|controlmaster (true|auto|ask|autoask))$' "$connection/config.tmp"; then
  rm -rf "$connection"
  exec "$binary" "$@"
fi
# Noninteractive scripts and pipelines are not terminal connections.
if grep -q '^requesttty auto$' "$connection/config.tmp" && { [ ! -t 0 ] || [ ! -t 1 ]; }; then
  rm -rf "$connection"
  exec "$binary" "$@"
fi
mv "$connection/config.tmp" "$connection/config"
# Persist lets the original foreground ssh return when its shell exits even
# while SFTP channels exist. The trap then terminates this owned master.
trap '"$binary" -F /dev/null -S "$connection/socket" -O exit localhost >/dev/null 2>&1; rm -rf "$connection"' EXIT
"$binary" -o ControlMaster=yes -o ControlPath="$connection/socket" -o ControlPersist=60 "$@"
status=$?
exit "$status"

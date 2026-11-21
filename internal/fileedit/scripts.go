package fileedit

const (
	scriptMarkerBegin = "\x02AIST_FILEEDIT_BEGIN\x02"
	scriptMarkerEnd   = "\x02AIST_FILEEDIT_END\x02"
)

const inspectScript = `
set -eu
raw="$1"
max_bytes="$2"
marked="$3"

emit_begin() {
  if [ "$marked" = "1" ]; then
    printf '\002AIST_FILEEDIT_BEGIN\002\n'
  fi
}

emit_end() {
  if [ "$marked" = "1" ]; then
    printf '\002AIST_FILEEDIT_END\002\n'
  fi
}

case "$raw" in
  "~")
    p="$HOME"
    ;;
  "~/"*)
    p="$HOME/${raw#\~/}"
    ;;
  /*)
    p="$raw"
    ;;
  *)
    echo "AIST_ERR:UNSAFE_PATH:path must be absolute or start with ~/" >&2
    exit 20
    ;;
esac

case "$p" in
  /proc|/proc/*|/sys|/sys/*|/dev|/dev/*)
    echo "AIST_ERR:UNSAFE_PATH:special filesystem path $p" >&2
    exit 20
    ;;
esac

if [ -L "$p" ]; then
  echo "AIST_ERR:SYMLINK:$p is a symlink" >&2
  exit 21
fi

parent=$(dirname "$p")
if [ ! -e "$p" ]; then
  if ! ls -ld "$parent" >/dev/null 2>&1; then
    if [ -e "$parent" ] || [ -L "$parent" ]; then
      echo "AIST_ERR:PERM:$parent" >&2
      exit 22
    fi
    anc="$parent"
    while [ "$anc" != "/" ] && [ "$anc" != "." ]; do
      if [ -d "$anc" ] && [ ! -x "$anc" ]; then
        echo "AIST_ERR:PERM:$anc" >&2
        exit 22
      fi
      anc=$(dirname "$anc")
    done
    echo "AIST_ERR:NO_PARENT:parent directory $parent is not accessible" >&2
    exit 23
  fi
  if [ ! -d "$parent" ]; then
    echo "AIST_ERR:NOT_DIR_PARENT:parent $parent is not a directory" >&2
    exit 21
  fi
  writable=0
  if [ -w "$parent" ] && [ -x "$parent" ]; then
    writable=1
  fi
  emit_begin
  printf 'STATUS=NEW\nPATH=%s\nWRITABLE=%s\n' "$p" "$writable"
  emit_end
  exit 0
fi

if [ ! -f "$p" ]; then
  echo "AIST_ERR:NOT_REGULAR:$p is not a regular file" >&2
  exit 21
fi

if [ ! -r "$p" ]; then
  echo "AIST_ERR:PERM:$p" >&2
  exit 22
fi

mode=$(stat -c '%a' "$p" 2>/dev/null || stat -f '%Lp' "$p")
size=$(wc -c < "$p" | tr -d '[:space:]')
if [ "$size" -gt "$max_bytes" ]; then
  echo "AIST_ERR:TOO_LARGE:$size bytes exceeds $max_bytes" >&2
  exit 24
fi

my_uid=$(id -u)
file_uid=$(stat -c '%u' "$p" 2>/dev/null || stat -f '%u' "$p")
writable=0
if [ "$my_uid" = "0" ]; then
  writable=1
elif [ "$file_uid" = "$my_uid" ] && [ -w "$parent" ] && [ -x "$parent" ]; then
  writable=1
fi

emit_begin
printf 'STATUS=EXISTS\nPATH=%s\nMODE=%s\nWRITABLE=%s\nDATA_BEGIN\n' "$p" "$mode" "$writable"
base64 < "$p" | tr -d '\r\n'
printf '\nDATA_END\n'
emit_end
`

const stageScript = `
set -eu
expected_len="$1"
stage=$(mktemp "${TMPDIR:-/tmp}/.aist-edit.XXXXXX")
chmod 600 "$stage"
if ! cat > "$stage"; then
  rm -f "$stage"
  exit 1
fi
actual_len=$(wc -c < "$stage" | tr -d '[:space:]')
if [ "$actual_len" != "$expected_len" ]; then
  rm -f "$stage"
  echo "AIST_ERR:TRUNCATED:staged $actual_len bytes, expected $expected_len" >&2
  exit 25
fi
printf '%s\n' "$stage"
`

const applyScript = `
set -eu
dst="$1"
stage="$2"
exists_flag="$3"
expected_hash="$4"
mode="$5"
expected_len="$6"

tmp=""
cleanup() {
  if [ -n "$tmp" ]; then
    rm -f "$tmp"
  fi
  if [ "$stage" != "-" ] && [ -n "$stage" ]; then
    rm -f "$stage"
  fi
}
trap cleanup EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    openssl dgst -sha256 "$1" | awk '{print $NF}'
  fi
}

if [ -L "$dst" ]; then
  echo "AIST_ERR:SYMLINK:$dst is a symlink" >&2
  exit 21
fi

parent=$(dirname "$dst")
owner=""
if [ "$exists_flag" = "1" ]; then
  if [ ! -f "$dst" ]; then
    echo "AIST_ERR:CONCURRENT:target file $dst no longer exists" >&2
    exit 30
  fi
  cur_hash=$(sha256_file "$dst")
  if [ "$cur_hash" != "$expected_hash" ]; then
    echo "AIST_ERR:CONCURRENT:file changed after the diff was prepared; no changes were written" >&2
    exit 30
  fi
  owner=$(stat -c '%u:%g' "$dst" 2>/dev/null || stat -f '%u:%g' "$dst" 2>/dev/null || true)
else
  if [ -e "$dst" ] || [ -L "$dst" ]; then
    echo "AIST_ERR:ALREADY_EXISTS:file already exists; no changes were written" >&2
    exit 31
  fi
  if [ ! -d "$parent" ]; then
    echo "AIST_ERR:NO_PARENT:parent directory $parent is not accessible" >&2
    exit 23
  fi
fi

tmp=$(mktemp "${parent}/.aist-write.XXXXXX")
if [ "$stage" = "-" ]; then
  cat > "$tmp"
else
  cat "$stage" > "$tmp"
fi

actual_len=$(wc -c < "$tmp" | tr -d '[:space:]')
if [ "$actual_len" != "$expected_len" ]; then
  echo "AIST_ERR:TRUNCATED:wrote $actual_len bytes, expected $expected_len" >&2
  exit 25
fi

if [ -n "$owner" ]; then
  chown "$owner" "$tmp" 2>/dev/null || true
fi
chmod "$mode" "$tmp"

mv -f "$tmp" "$dst"
tmp=""
`

# Rune Scroll Custom Codec

This example teaches SRouter a tiny, whimsical wire format that is neither JSON
nor protobuf. The application handler still receives and returns normal,
type-safe Go structs; `RuneScrollCodec` owns all transport details.

An incoming scroll looks like this:

```text
SROUTER-RUNE-SCROLL/1
seeker :: Ada Lovelace
question :: Will custom codecs make APIs more fun?
vibe :: electric
```

The response is another custom-formatted scroll:

```text
SROUTER-RUNE-SCROLL/1
✦ seeker :: Ada Lovelace
✦ omen :: The types align in your favor
✦ aura :: electric
✦ lucky-rune :: ᚱ
✦ echoes :: Will custom codecs make APIs more fun?
```

The omen and rune are selected deterministically from the question and vibe.

## Run it

From the repository root:

```bash
go run ./examples/custom-codec
```

Send a scroll as the request body. This exercises `Decode` and `Encode`:

```bash
curl --data-binary @- http://localhost:8080/oracle <<'SCROLL'
SROUTER-RUNE-SCROLL/1
seeker :: Ada Lovelace
question :: Will custom codecs make APIs more fun?
vibe :: electric
SCROLL
```

Or let SRouter extract and base64-decode a query parameter before handing its
bytes to the codec. This exercises `DecodeBytes` and `Encode`:

```bash
scroll="$(printf '%s' 'SROUTER-RUNE-SCROLL/1
seeker :: Grace Hopper
question :: Will this work from a query parameter?
vibe :: curious' | base64 | tr -d '\n')"
curl --get --data-urlencode "scroll=${scroll}" http://localhost:8080/oracle
```

## What to notice

- `RuneScrollCodec` implements `codec.Codec[OracleRequest, OracleResponse]` and
  includes a compile-time assertion to prove it.
- The POST route uses `Decode`; the GET route uses `DecodeBytes` after SRouter
  handles the base64 transport layer.
- Both routes share the same typed handler and sanitizer.
- The handler is completely unaware of the wire format.
- The codec controls the successful response's custom `Content-Type`. Framework
  errors remain SRouter's standard JSON error responses.

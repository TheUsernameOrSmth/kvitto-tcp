# kvitto-tcp

A Go service that receives receipt printer data over TCP, parses the receipts,
and publishes JSON to MQTT. It handles printer control bytes and Swedish
characters, extracting the receipt number, order time, seller, and sold
products.

## Running

Install the Go version required by [go.mod](go.mod) and run from the repository
root:

```sh
go run .
```

The current connection settings are defined in [main.go](main.go) and
[types.go](types.go):

| Setting      | Value                              |
|--------------|------------------------------------|
| TCP listener | `0.0.0.0:9100`                     |
| MQTT broker  | `tcp://server.insektionen.se:1883` |
| MQTT topic   | `kistan/bong`                      |

Note: the MQTT topic can be overridden with the ENV variable `MQTT_TOPIC`

The service connects to MQTT before opening the TCP listener. Configure the
receipt source to send printer data to this machine on port `9100`.

Each JSON payload contains `timestamp` (Unix seconds), `number`, `sold_by`, and
a `sold` array with `category`, `name`, `count`, and `extra_text`. Receipt times
use the `Europe/Stockholm` timezone and the current year, since the source
receipts omit the year.

## Samples

The [samples](samples) directory contains captured receipt data:

| File                         | Contents                                                                                                           |
|------------------------------|--------------------------------------------------------------------------------------------------------------------|
| [dump.bin](samples/dump.bin) | Raw printer bytes for receipt `225555`, including control commands and NUL bytes.                                  |
| [hex.txt](samples/hex.txt)   | Hex-encoded samples, with one complete receipt/message per row.                                                    |

The first hex row is identical to `dump.bin`. The sample test sends raw printer data
through the TCP handler and passes decoded text directly to the receipt parser.

## Capturing a sample

With `nc` (netcat) and `xxd` installed, stop the service to free port `9100`,
then run this from the repository root:

```sh
{ printf '\n'; nc -l 9100 | xxd -p -c 0; } >> samples/hex.txt
```

Point the receipt source to the capture machine's IP
address and port `9100`. Send one complete receipt, then close the sender's
connection to finish the capture. Each run appends one hex row directly to
`samples/hex.txt`. The leading newline keeps entries separate; blank rows are
ignored by the test.

Run the sample test below to inspect the generated JSON.

## Tests

The sample test uses the `samples` build tag, so ordinary test runs exclude it:

```sh
go test ./...
```

Run all entries in `samples/hex.txt` and print their formatted JSON:

```sh
go test -tags samples -run '^TestSampleHexJSON$' -v -count=1 ./...
```

Each nonempty row becomes a subtest named `row_1`, `row_2`, and so on, using its
line number in the file. The test checks that each entry produces exactly one
MQTT payload containing valid JSON. MQTT publishing is captured locally; no
broker or running service is required. `-v` shows the JSON and `-count=1` reruns
the test without using cached results.

To inspect just the second row:

```sh
go test -tags samples -run '^TestSampleHexJSON$/^row_2$' -v -count=1 ./...
```

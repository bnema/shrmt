# Beyonder protocol (NVIDIA SHIELD TV remote, port 8987)

Status: reconstructed from static analysis of com.nvidia.shield.remote 2.1.8
(Blutter Dart AOT disassembly + jadx). All values are static reconstructions; nothing here was verified against a live
SHIELD yet. See Unknowns.

## 1. Transport

- TCP port 8987 (mDNS `_nv_shield_remote._tcp`), wrapped in TLS. The Dart side
  never touches TLS; it calls a platform MethodChannel
  `com.nvidia.beyonder_backend/methods` (`tcp_socket/new_instance` with
  `host`,`port`,`androidId`; `tcp_socket/connect`; `tcp_socket/send` with
  `socketId`,`data`) — `jadx v2/a.java c()`, Dart `tcp/TcpSocket.dart`
  `TcpSocket::send` @0x5178b0 / invokeMethod @0x51794c.
- Socket creation: `jadx v2/b.java a()` (CertificateManager.createClientSocket):
  `SSLContext.getInstance("TLS")`; trust manager is a *permissive*
  `X509TrustManager` (`v2/b.java` inner `C0125b`, all checks empty) → the
  SHIELD's self-signed server cert is accepted without verification.
  `setTcpNoDelay(true)`, `setTrafficClass(16)`, connect timeout 5000 ms.
- Client certificate: KeyManagers are built from a per-host Android keystore
  file `"ClientKeyStore" + androidId`, alias `"nvbeyonder"`, empty password
  (`v2/b.java f()`, `d()`, `e()`, `g()`; `f7165a = new char[0]`). If the
  keystore is empty (first pairing) **no client certificate is presented**
  (`keyManagerArrD = null`). The key is RSA, stored as PKCS#8
  (`v2/b.java c()`), cert X.509 DER (`b()`).
- Where the client cert comes from: the SHIELD *issues* it. `PairingResult`
  carries `privateKey` (field 3) and `certificate` (field 4); the app stores
  them via `cert_manager/store_cert` (`v2/a.java c()` → `v2/b.o()`), see §4.
- `v2/c.java m()`: `setNeedClientAuth(true)` (meaningless on a client socket),
  `setUseClientMode(true)`, `setKeepAlive(true)`, `startHandshake()`.
- Reads: `v2/c.java` listener thread reads up to 65536 bytes per `read()` and
  forwards each chunk raw to Dart (`tcp_socket/callback_stream/data_<id>`).
  Writes: `v2/c.java` handler `what==3` writes the byte[] as-is + flush.
  **The Java layer adds no length prefix and does no reassembly.**

## 2. Framing

**There is no length prefix and no varint delimiter.** Each message is one
serialized `BaseMessage` written as-is to the TLS stream.

Evidence:
- Send path: `NetworkDevice::sendMessage` @0x7d1d50 → `ClientConnector::sendMessage`
  @0x516ab0 (named arg `msgFraming`) → `NativeNetConnector::sendMessageImpl`
  @0x7d152c, which calls `TcpSocket::send(data)` and does **not** use
  `msgFraming` → `tcp_socket/send` → `v2/c.java` writes the bytes verbatim.
- Encoders return `GeneratedMessage::writeToBuffer()` output directly
  (`NvBeyonderBaseMsg::buildBaseMsg` @0x51aaf0).

Large messages ("message framing"):
- `NetworkDevice::sendMessage` @0x7d1d50: if `len(msg) > 0x800` (2048 bytes),
  it first sends `encodeIndicatingSizeMsg(len)` @0x525b20 →
  `NvBeyonderBaseMsg::buildIndicatingSizeMsg` @0x51ac14 = `BaseMessage{service_id=0,
  size=len}`, then sends the full message bytes (both with `msgFraming: true`,
  which is ignored below Dart). Messages ≤ 2048 bytes are sent alone.
  No capability check is done on this path in `sendMessage` (the
  `MESSAGE_FRAMING`=128 capability bit is advertised in the hello).
- Receive: `MessageParser::parse` @0x519134. If not framing, each received
  chunk (one `read()` from Java, ≤ 65536 bytes) is parsed with
  `BaseMessage.fromBuffer`. If it has `timeStamp` → RTT reply. Else if
  `size > 0` → `_startMsgFraming(size)` @0x51db08 allocates a buffer; next
  chunks are appended by `_doFraming` @0x51dc14 until exactly `size` bytes
  (`FramingResult` COMPLETED=0 / INCOMPLETE=1 / ERROR_NOT_START_FRAMING=2 /
  ERROR_SIZE_INCORRECT=3), then parsed as one `BaseMessage`.
- Consequence: the app relies on **one TLS read == one message** for small
  messages. A reimplementation should write each message in a single TLS
  write and, on receive, treat each TLS record/read as one message
  (protobuf has no self-delimiting, so coalesced reads would misparse).

## 3. BaseMessage envelope

`internal/beyonder/proto/base.proto`. From `BaseMessage._i` @0x50927c
(`base.pb.dart`):

| tag | name | type (PbFieldType) | notes |
|---|---|---|---|
| 1 | serviceId | enum `BaseMessage_ServiceId` (`e()`) | selects payload type |
| 2 | payload | bytes (0x20) | serialized service message |
| 3 | serviceNumber | int32 (0x800) | never set by app |
| 4 | size | int32 (0x800) | framing announcement (§2) |
| 5 | timeStamp | uint64 (0x10000) | RTT probe |

ServiceId (objs.txt `Obj!BaseMessage_ServiceId@…`): 0 PROTO3_REQUIRES_ZERO,
1 HANDSHAKE, 2 CATALOG, 10 AUTHENTICATION, 11 HOST_INFO, 1001 VIRTUAL_INPUT,
1002 AUDIO_INPUT, 1003 FILE_TRANSFER, 1004 INPUT_METHOD_MONITOR,
1005 MEDIA_SESSION, 1006 APP_INSTALL, 1007 FEEDBACK, 1008 VOLUME_CONTROL,
1009 REMOTE_LAUNCHER, 1010 AUDIO_OUTPUT, 1011 ACCESSORY_LOCATOR,
2000 CATEGORY_INTENT.

`buildBaseMsg(serviceId, payload)` @0x51aaf0 sets tag 1 and tag 2 only
(returns null if serviceId ≤ 0 or payload empty). There is no request id /
sequence number. Per-service payloads carry their own `command` enum.

Encoder → service mapping (`MessageEncoder.dart`): encodeHandshakeHello
@0x525b58 → 1; encodeAuthenticationPinRequest @0x525830 and
encodeAuthenticationPinCodeSend @0x526390 → 10; encodeHostInfoRequestCommand
@0x525a2c → 11; all encodeVIS* (incl. encodeVISSpecialInputCommand @0x526414)
→ 1001; encodeRequestSyncText → 1004; AIS → 1002; AOS → 1010; Feedback → 1007;
AccessoryLocator → 1011.

## 4. Connection sequence

Schemas: `handshake.proto`, `authentication.proto`, `host_info.proto`,
`capability.proto`.

1. TLS connect to :8987 (client cert only if already paired, §1).
2. Client → `BaseMessage{1: HANDSHAKE, 2: Handshake}`:
   `NetworkDevice::handshakeSayHello` @0x7d1948 → `encodeHandshakeHello`
   @0x525b58 → `NvBeyonderHandshakeMsg::buildHelloMsg` @0x525c24:
   - `command`(1) = COMMAND_HELLO (1)
   - `name`(2) = client device name (hello not built if empty)
   - `deviceOs`(3) = DEVICEOS_ANDROID (1) on Android, IOS (2) otherwise
   - `capability`(5) = `CapabilityUtils.CLIENT_ALL_CAPABILITY`
     (@0x534710 loads Smi 0x27ff6 → **81915 = 0x13FFB**: all bits 1..65536
     except INPUT_ABSOLUTE(4), PRIVATE_LISTENING(16384),
     ACCESSORY_LOCATOR(32768)). BLE uses `CLIENT_BLE_CAPABILITY` = 42691.
   - `certificate`(4): only if a `cert:` arg is given; NetworkDevice passes null.
   - Handshake (Smi-decoded) also lists `COMMAND_IDENTIFY`=2, never sent.
   - Capability enum: NONE 0, INPUT_GENERIC 1, INPUT_METHOD 2, INPUT_ABSOLUTE 4,
     FILE_TRANSFER 8, VOICE_SEARCH 16, MEDIA_CONTROL 32, HOST_INFO 64,
     MESSAGE_FRAMING 128, APP_INSTALL 256, PIN_CODE_CANCEL 512,
     RTT_MESSAGING 1024, FEEDBACK 2048, VOLUME_CONTROL 4096,
     REMOTE_LAUNCHER 8192, PRIVATE_LISTENING 16384, ACCESSORY_LOCATOR 32768,
     ACCESSORY_LISTENER 65536.
3. Server → `BaseMessage{1: AUTHENTICATION, 2: AuthenticationServicePayload{
   command=COMMAND_AUTHORIZATION_LEVEL(1000), object=AuthorizationLevel}}`.
   The parser has **no** handler for service HANDSHAKE (no reference to
   ServiceId@83e6d1 in `MessageParser.dart`), so the hello reply is this
   message. `_handleCommandAuthenticationLevel` @0x519b28 reads
   `level`(1), `deviceName`(2), `capability`(3) = server capability bitmask.
   `BaseDevice::_onAuthenticationLevel` @0x5347e8 stores it, and if the server
   has RTT_MESSAGING (1024) starts the RTT timer (network only).
4. App decision (`ShieldRemote/util/backend_utils.dart`
   `DeviceListenerImpl::onAuthenticationLevel` @0x5349ac, `cmp x1,#0x3e8`
   @0x534bf4): level == LEVEL_VIRTUAL_INPUT (1000) → already paired
   (`_onPairedSuccess`), ready for input. Otherwise (LEVEL_NONE=1) →
   `authenticationPinRequest`.
5. Client → `AuthenticationServicePayload{command=COMMAND_PIN_REQUEST(1101)}`
   (no object): `buildPinRequestMsg` @0x525870 (uses AuthServicePayload
   CommandType@83e791 = 1101).
6. Server → `command=COMMAND_PIN_CHALLENGE(1103)` (TV shows a PIN);
   `BaseDevice::_onAuthenticationPinChallenge` → UI prompt.
7. Client → `command=COMMAND_PIN_CODE(1105), object=PairingPinCode{
   pinCode(1), deviceId(2)}`: `buildPinCodeSendMsg` @0x525960 /
   `_buildPinCode` @0x5258d0 (setters at index 0 and 1 → tags 1 and 2).
8. Server → `command=COMMAND_PAIRING_RESULT(2101), object=PairingResult{
   resultType(1), resultString(2), privateKey(3), certificate(4)}`
   (`_handleCommandPairResult` @0x519698). On RESULT_SUCCESS with non-empty
   key+cert (`_handleAuthResultSuccess` @0x519808; otherwise reported as
   RESULT_CERTIFICATE_FAILURE "server side error"),
   `BaseDevice::_onAuthenticationPairingResult` calls
   `CertificateManager::storeClientCertificate` @0x4fe974 → Java
   `cert_manager/store_cert` → keystore `ClientKeyStore<androidId>` alias
   `nvbeyonder`. So **the PIN authorizes the SHIELD to issue a client
   certificate**; later connections present that cert in TLS and the server
   answers the hello with LEVEL_VIRTUAL_INPUT.
   Then `_onPairedSuccess` → `requestHostInfo` (HOST_INFO 11,
   `HostInfoServicePayload{command=1101}` → reply 2101 + HostInformation).
9. RTT keepalive (only if server capability has RTT_MESSAGING):
   `BaseDevice::_startRttTimer` @0x534d48: `Timer.periodic`, period 3 s in
   release builds, 1 s otherwise (`isReleaseMode` → tagged Smi 6 or 2 at
   @0x534d7c..0x534d8c). Each tick (closure @0x534eb8) does nothing unless
   ≥5000 ms have passed since a stored timestamp (field_3b, presumably the
   last RTT send/reply); then it logs the consecutive-loss counter, disconnects
   when it reaches 5, else sends a probe via `_sendRttPacket` @0x535088 → `encodeRttPacketMsg` @0x524bf0 →
   `buildTimeStampMsg(ms)` @0x51ab90 = `BaseMessage{serviceId=0,
   timeStamp=now_ms}`. Replies carrying `timeStamp` are routed to
   `_onReceiveRttMessage` @0x522b74 (RTT = now − timeStamp, averaged over 10).

## 5. VirtualInput

Schema: `virtual_input.proto`. `VirtualInputPayload._i` @0x50e8c4:

| tag | name | type |
|---|---|---|
| 1 | request | enum Request |
| 2 | accessoryType | enum AccessoryType |
| 4 | inputType | enum InputType |
| 5 | keyState | enum KeyState |
| 6 | value | repeated sint32, packed (`BuilderInfo.p` @0x50eb04 → type 0x2006, `_checkSigned32`) |
| 7 | text | string |
| 8 | specialInput | enum SpecialInput |

(no tag 3.) Enums (objs.txt): Request {0, ALLOCATE 10, FREE 11, SET_RANGE 12,
COMMAND 20}; AccessoryType {0, KEYBOARD 1, MOUSE 2, TOUCHPAD 3, TOUCHSCREEN 4,
CONTROLLER 5, REMOTE 6}; InputType {0, LINUX_EVENT 1, LINUX_EVENT_STATE 2,
ASCII_KEY_STATE 10, ASCII_KEY 11, ASCII_STRING 12, RELATIVE 20, ABSOLUTE 21,
WHEEL 22, SPECIAL 30}; KeyState {0, KEY_DOWN 1, KEY_UP 2}; SpecialInput {0,
TOGGLE_OSC 10, HOME 11, BACK 12, SETTINGS 20, POWER_MENU 21, SLEEP_NOW 30,
GOOGLE_ASSIST 31}; plus unused KeyActions {UP 0, DOWN 1}, KeyCodes
{VOLUME_DOWN 114, VOLUME_UP 115}.

`buildSpecialInputCommandMsg(int)` @0x524dcc sets:
`request`=REQUEST_COMMAND (20, Obj@83da51), `accessoryType`=ACCESSORY_CONTROLLER
(5, Obj@83d971), `inputType`=INPUT_SPECIAL (30, Obj@83d831),
`specialInput`=SpecialInput.valueOf(arg). Called from
`BaseDevice::virtualSpecialInputCommand` @0x536e28 (no capability check),
invoked by the UI at `backend_utils.dart` @0x5435d4.

Other builders, for reference: `buildKeyCodeMsg` @0x5250cc = {COMMAND,
KEYBOARD, LINUX_EVENT, value=[keycode]}; `buildKeyActionMsg` @0x524f60 =
{COMMAND, KEYBOARD, ASCII_KEY_STATE, keyState = arg==0 ? KEY_UP : KEY_DOWN,
value=[key]}.

## 6. Worked example (derived, not captured)

Power toggle = special input 30 (ACTION_SLEEP_NOW). Encoded with `protoc
--encode` against the reconstructed schemas:

```
VirtualInputPayload  = 08 14  10 05  20 1e  40 1e
                       req=20 acc=5  in=30  special=30
BaseMessage (frame)  = 08 e9 07  12 08  08 14 10 05 20 1e 40 1e
                       svc=1001  payload(len 8)
```

Full bytes written to the TLS stream (13 bytes, no prefix):
`08e907120808141005201e401e`

Other derived frames:
- Hello, name "shrmt", Android, caps 81915:
  `0801120f080112057368726d74180128fbff04`
- PIN request: `080a120308cd08`
- RTT probe: `28 <varint ms>` (plus possibly `08 00`, see Unknowns)

## 7. Unknowns

- **Default-valued fields on the wire.** Dart `protobuf` `setField` with a
  zero enum (RTT / size messages set serviceId=PROTO3_REQUIRES_ZERO) may be
  serialized as `08 00` by the Dart runtime; not verified. Irrelevant for the
  power frame (no zero fields).
- **Is pairing mandatory for VIRTUAL_INPUT?** Static evidence says the app
  only sends input after LEVEL_VIRTUAL_INPUT; whether the server rejects
  VIRTUAL_INPUT at LEVEL_NONE is unverified. Prior probing (EOF on
  service_id ≥ 2 without auth) suggests yes.
- **Does the authorization level upgrade on the same connection after
  PAIRING_RESULT success**, or does the client need to reconnect with the
  new cert? `_onPairedSuccess` proceeds to requestHostInfo on the same
  connection, suggesting the session is already authorized, but unconfirmed.
- **PairingPinCode.deviceId value**: argument source of `_buildPinCode`
  @0x5258d0 not traced (likely androidId or a client id).
- **Who calls `handshakeSayHello`**: invoked via dynamic dispatch (no direct
  `bl #0x7d1948` found); assumed on TCP connected.
- **Hello `name` exact value** used by the app (device model vs. user name)
  not traced; any non-empty string is expected to work.
- **Client cert requirements** (key size, subject) when the server issues it:
  produced server-side; client stores whatever it gets (RSA PKCS#8 + X.509).
- **Message coalescing**: behaviour if the server coalesces two messages in
  one TLS record is undefined in the app; reimplementation should tolerate
  it defensively (e.g. try parsing, but no delimiter exists).
- `serviceNumber` (tag 3), `COMMAND_IDENTIFY`, `COMMAND_UNPAIR`,
  `SERVICEID_CATALOG`/`CATEGORY_INTENT`: semantics unknown, unused by the
  traced paths.
- Original `.proto` package name: unknown (`PackageName` object not
  decoded); schemas use `package beyonder`.

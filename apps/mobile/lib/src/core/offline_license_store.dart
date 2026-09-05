import 'dart:convert';
import 'dart:typed_data';
import 'package:encrypt/encrypt.dart' as enc;

/// OfflineLicenseStore keeps download metadata encrypted at rest.
/// Mirrors server/internal/offline Seal/Open (AES-GCM 256).
/// Key is derived from user secret; plaintext is never written.
class OfflineLicenseStore {
  OfflineLicenseStore({required Uint8List keyBytes}) {
    if (keyBytes.length != 32) throw ArgumentError('key must be 32 bytes');
    _key = enc.Key(keyBytes);
  }
  late final enc.Key _key;

  /// Encrypt a JSON-serialisable licence map. Returns base64(nonce+ciphertext).
  String seal(Map<String, dynamic> licence) {
    final plain = utf8.encode(jsonEncode(licence));
    final iv = enc.IV.fromSecureRandom(12); // 96-bit nonce for GCM
    final encrypter = enc.Encrypter(enc.AES(_key, mode: enc.AESMode.gcm));
    final ct = encrypter.encryptBytes(plain, iv: iv);
    final combined = Uint8List.fromList(iv.bytes + ct.bytes);
    return base64Encode(combined);
  }

  Map<String, dynamic> open(String sealed) {
    final bytes = base64Decode(sealed);
    if (bytes.length < 12) throw FormatException('ciphertext too short');
    final iv = enc.IV(bytes.sublist(0, 12));
    final ct = enc.Encrypted(bytes.sublist(12));
    final encrypter = enc.Encrypter(enc.AES(_key, mode: enc.AESMode.gcm));
    final plain = encrypter.decryptBytes(ct, iv: iv);
    return jsonDecode(utf8.decode(plain)) as Map<String, dynamic>;
  }

  /// Licence expiry guard — same rule as server offline.Licence.IsExpired.
  static bool isExpired(Map<String, dynamic> licence, DateTime now) {
    final exp = DateTime.tryParse(licence['expires_at'] as String? ?? '');
    return exp == null || now.isAfter(exp);
  }

  static bool needsRenewal(Map<String, dynamic> licence, DateTime now) {
    final exp = DateTime.tryParse(licence['expires_at'] as String? ?? '');
    if (exp == null) return true;
    return exp.difference(now).inHours < 24;
  }
}

import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:cryptography/cryptography.dart';
import 'package:iconfess_api/iconfess_api.dart';

/// Remove downloads written by older releases to the Documents directory.
/// This runs at app startup without needing a Keychain unlock, so plaintext
/// does not wait for the user to open the Bible reader before cleanup.
Future<void> purgeLegacyOfflineBibleDownloads(Directory documentsDirectory) async {
  final oldDirectory = Directory('${documentsDirectory.path}/bible-offline');
  if (await oldDirectory.exists()) await oldDirectory.delete(recursive: true);
}

/// Encrypted, account-bound offline Bible packages. The server's signed URL
/// transports bytes; it is not a license and must not become a plaintext cache.
/// Metadata and each package's random AES-256-GCM key live in the device
/// Keychain/Keystore. Only ciphertext (nonce | body | tag) is written to disk.
final class OfflineBiblePackageStore {
  OfflineBiblePackageStore(this._storage, this._directory);

  static const format = 'aes-256-gcm-v1';
  static const _nonceLength = 12;
  static const _tagLength = 16;
  static const _maxPlaintextBytes = 32 * 1024 * 1024;
  static final _cipher = AesGcm.with256bits();

  final SecureStorage _storage;
  final Directory _directory;

  static String metadataKey(String translationID, String bookID) =>
      'bible.offline.$translationID.$bookID';

  static List<int> _associatedData(Map<String, dynamic> metadata) => utf8.encode(
    jsonEncode([
      format,
      metadata['owner_user_id'],
      metadata['translation_id'],
      metadata['book_id'],
      metadata['license_id'],
      metadata['expires_at'],
      metadata['content_hash'],
    ]),
  );

  Future<void> save({
    required String translationID,
    required String bookID,
    required String ownerID,
    required Map<String, dynamic> manifest,
    required List<int> bytes,
  }) async {
    final hash = sha256.convert(bytes).toString();
    final expiry = DateTime.tryParse(manifest['offline_expires_at'] as String? ?? '');
    final licenseID = manifest['license_id'] as String?;
    if (bytes.length > _maxPlaintextBytes || hash != manifest['content_hash'] || expiry == null ||
        !expiry.isAfter(DateTime.now().toUtc()) ||
        licenseID == null || licenseID.isEmpty || ownerID.isEmpty) {
      throw const FormatException('Invalid offline license or package checksum');
    }
    final key = metadataKey(translationID, bookID);
    final secret = await _cipher.newSecretKey();
    final keyBytes = await secret.extractBytes();
    final metadata = <String, dynamic>{
      'format': format,
      'translation_id': translationID,
      'book_id': bookID,
      'owner_user_id': ownerID,
      'license_id': licenseID,
      'expires_at': expiry.toUtc().toIso8601String(),
      'content_hash': hash,
      'encryption_key': base64Encode(keyBytes),
      'attribution_required': manifest['attribution_required'],
      'attribution_text': manifest['attribution_text'],
    };
    final nonce = _cipher.newNonce();
    final box = await _cipher.encrypt(
      bytes, secretKey: secret, nonce: nonce, aad: _associatedData(metadata),
    );
    // File names are hashes of fixed server-provided identity and random nonce;
    // a provider book name cannot escape the private application directory.
    final filename = sha256.convert(utf8.encode('$ownerID|$translationID|$bookID|$licenseID|${base64Encode(nonce)}')).toString();
    await _directory.create(recursive: true);
    final file = File('${_directory.path}/$filename.bin');
    final oldMetadata = await _storage.read(key);
    await file.writeAsBytes([...nonce, ...box.cipherText, ...box.mac.bytes], flush: true);
    metadata['path'] = file.path;
    try {
      await _storage.write(key, jsonEncode(metadata));
    } catch (_) {
      await file.delete();
      rethrow;
    }
    // Remove the previous encrypted package or an older plaintext .json file
    // only after the new key and metadata have been committed to secure storage.
    if (oldMetadata != null) {
      try {
        final previous = jsonDecode(oldMetadata) as Map<String, dynamic>;
        final path = previous['path'] as String?;
        if (path != null && path != file.path) {
          final oldFile = File(path);
          if (await oldFile.exists()) await oldFile.delete();
        }
      } catch (_) {
        // The new package is valid even when a former file is already missing.
      }
    }
  }

  Future<OfflineBiblePackage?> load({
    required String translationID,
    required String bookID,
    required String ownerID,
  }) async {
    final key = metadataKey(translationID, bookID);
    final raw = await _storage.read(key);
    if (raw == null) return null;
    try {
      final metadata = jsonDecode(raw) as Map<String, dynamic>;
      // Never read historical plaintext packages. Delete those on first use.
      if (metadata['format'] != format) {
        await remove(key);
        return null;
      }
      if (ownerID.isEmpty || metadata['owner_user_id'] != ownerID ||
          metadata['translation_id'] != translationID || metadata['book_id'] != bookID) {
        return null;
      }
      final expiry = DateTime.tryParse(metadata['expires_at'] as String? ?? '');
      if (expiry == null || !expiry.isAfter(DateTime.now().toUtc())) {
        await remove(key);
        return null;
      }
      final encodedKey = base64Decode(metadata['encryption_key'] as String);
      if (encodedKey.length != 32) throw const FormatException('Invalid offline key');
      final path = metadata['path'] as String;
      final file = File(path);
      if (await file.length() > _maxPlaintextBytes + _nonceLength + _tagLength) {
        throw const FormatException('Offline package exceeds the size limit');
      }
      final encrypted = await file.readAsBytes();
      if (encrypted.length < _nonceLength + _tagLength ||
          encrypted.length > _maxPlaintextBytes + _nonceLength + _tagLength) {
        throw const FormatException('Incomplete offline package');
      }
      final plaintext = await _cipher.decrypt(
        SecretBox(
          encrypted.sublist(_nonceLength, encrypted.length - _tagLength),
          nonce: encrypted.sublist(0, _nonceLength),
          mac: Mac(encrypted.sublist(encrypted.length - _tagLength)),
        ),
        secretKey: SecretKey(encodedKey),
        aad: _associatedData(metadata),
      );
      if (sha256.convert(plaintext).toString() != metadata['content_hash']) {
        throw const FormatException('Offline package checksum mismatch');
      }
      final payload = jsonDecode(utf8.decode(plaintext)) as Map<String, dynamic>;
      final translation = payload['translation'] as Map?;
      final book = payload['book'] as Map?;
      if (translation?['id'] != translationID || book?['id'] != bookID) {
        throw const FormatException('Offline package identity mismatch');
      }
      return OfflineBiblePackage(payload, expiry);
    } catch (_) {
      // Missing, damaged or unauthenticated bytes must never be served. A
      // keystore READ failure is outside this catch and fails closed upstream.
      await remove(key);
      return null;
    }
  }

  Future<void> remove(String key) async {
    final raw = await _storage.read(key);
    if (raw == null) return;
    // Delete the key first: if file cleanup fails, no copy can be decrypted.
    await _storage.delete(key);
    try {
      final metadata = jsonDecode(raw) as Map<String, dynamic>;
      final path = metadata['path'] as String?;
      if (path != null) {
        final file = File(path);
        if (await file.exists()) await file.delete();
      }
    } on Exception {
      // The key is gone; an orphaned ciphertext file is inert.
    }
  }
}

final class OfflineBiblePackage {
  const OfflineBiblePackage(this.payload, this.expiresAt);
  final Map<String, dynamic> payload;
  final DateTime expiresAt;
}

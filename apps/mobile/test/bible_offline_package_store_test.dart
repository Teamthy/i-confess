import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/features/bible/offline_package_store.dart';
import 'package:iconfess_api/iconfess_api.dart';

final class _MemorySecureStorage implements SecureStorage {
  final values = <String, String>{};
  bool failWrites = false;
  @override
  Future<String?> read(String key) async => values[key];
  @override
  Future<void> write(String key, String value) async {
    if (failWrites) throw const FileSystemException('Keystore unavailable');
    values[key] = value;
  }
  @override
  Future<void> delete(String key) async { values.remove(key); }
}

void main() {
  late _MemorySecureStorage secure;
  late Directory directory;
  late OfflineBiblePackageStore store;
  const key = 'bible.offline.fixture.Gen';
  final payload = utf8.encode(jsonEncode({
    'translation': {'id': 'fixture'},
    'book': {'id': 'Gen'},
    'chapters': [
      {'chapter': 1, 'verses': [{'text': 'Fixture verse; not Scripture.'}]},
    ],
  }));
  Map<String, dynamic> manifest() => {
        'license_id': 'license-1',
        'content_hash': sha256.convert(payload).toString(),
        'offline_expires_at': DateTime.now().toUtc().add(const Duration(days: 2)).toIso8601String(),
        'attribution_required': false,
        'attribution_text': '',
      };

  setUp(() async {
    secure = _MemorySecureStorage();
    directory = await Directory.systemTemp.createTemp('iconfess-bible-offline-test-');
    store = OfflineBiblePackageStore(secure, directory);
  });
  tearDown(() async {
    if (await directory.exists()) await directory.delete(recursive: true);
  });

  test('a cold offline launch cannot mistake an old owner key for the new account', () {
    final claims = base64Url.encode(utf8.encode(jsonEncode({'sub': 'account-b'})));
    final token = 'e30.$claims.signature';
    expect(bibleSessionSubject(token), 'account-b');
    expect(bibleOfflineOwnerMatchesSession(
      token: token, signedInID: null, storedOwner: 'account-a',
    ), isFalse);
    expect(bibleOfflineOwnerMatchesSession(
      token: token, signedInID: 'account-a', storedOwner: 'account-b',
    ), isFalse);
    expect(bibleOfflineOwnerMatchesSession(
      token: token, signedInID: null, storedOwner: 'account-b',
    ), isTrue);
    expect(bibleOfflineOwnerMatchesSession(
      token: 'not-a-jwt', signedInID: null, storedOwner: 'account-b',
    ), isFalse);
  });

  test('startup cleanup removes only legacy Documents packages', () async {
    final legacy = Directory('${directory.path}/bible-offline');
    await legacy.create();
    await File('${legacy.path}/old.json').writeAsBytes(payload);
    final unrelated = File('${directory.path}/other-data.json');
    await unrelated.writeAsString('keep');
    await purgeLegacyOfflineBibleDownloads(directory);
    expect(await legacy.exists(), isFalse);
    expect(await unrelated.readAsString(), 'keep');
  });

  test('encrypts at rest, verifies identity and rotates the package key', () async {
    await store.save(
      translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1',
      manifest: manifest(), bytes: payload,
    );
    final first = jsonDecode((await secure.read(key))!) as Map<String, dynamic>;
    final encrypted = await File(first['path'] as String).readAsBytes();
    expect(utf8.decode(encrypted, allowMalformed: true), isNot(contains('Fixture verse')));
    expect(encrypted, isNot(orderedEquals(payload)));
    expect(first['encryption_key'], isNotNull);
    expect((await store.load(translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1'))?.payload['chapters'], isNotEmpty);
    expect(await store.load(translationID: 'fixture', bookID: 'Gen', ownerID: 'other-user'), isNull);

    await store.save(
      translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1',
      manifest: manifest(), bytes: payload,
    );
    final second = jsonDecode((await secure.read(key))!) as Map<String, dynamic>;
    expect(second['encryption_key'], isNot(first['encryption_key']));
    expect(await File(first['path'] as String).exists(), isFalse);
    expect(await File(second['path'] as String).exists(), isTrue);
    await store.remove(key);
    expect(await File(second['path'] as String).exists(), isFalse);
    expect(await secure.read(key), isNull);
  });

  test('keystore write failure leaves no readable or orphaned file', () async {
    secure.failWrites = true;
    await expectLater(
      store.save(
        translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1',
        manifest: manifest(), bytes: payload,
      ),
      throwsA(isA<FileSystemException>()),
    );
    expect(await directory.list().toList(), isEmpty);
    expect(await secure.read(key), isNull);
  });

  test('tampering with ciphertext or license metadata fails closed', () async {
    await store.save(
      translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1',
      manifest: manifest(), bytes: payload,
    );
    var metadata = jsonDecode((await secure.read(key))!) as Map<String, dynamic>;
    final file = File(metadata['path'] as String);
    final ciphertext = await file.readAsBytes();
    ciphertext[15] ^= 1;
    await file.writeAsBytes(ciphertext);
    expect(await store.load(translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1'), isNull);
    expect(await secure.read(key), isNull);

    await store.save(
      translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1',
      manifest: manifest(), bytes: payload,
    );
    metadata = jsonDecode((await secure.read(key))!) as Map<String, dynamic>;
    metadata['expires_at'] = DateTime.now().toUtc().add(const Duration(days: 4)).toIso8601String();
    await secure.write(key, jsonEncode(metadata));
    expect(await store.load(translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1'), isNull);
    expect(await secure.read(key), isNull);
  });

  test('rejects expired or wrong-hash downloads and purges legacy plaintext', () async {
    final invalid = manifest()..['content_hash'] = '00';
    await expectLater(
      store.save(translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1', manifest: invalid, bytes: payload),
      throwsFormatException,
    );
    expect(await directory.list().toList(), isEmpty);
    final expired = manifest()
      ..['offline_expires_at'] = DateTime.now().toUtc().subtract(const Duration(hours: 1)).toIso8601String();
    await expectLater(
      store.save(translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1', manifest: expired, bytes: payload),
      throwsFormatException,
    );
    expect(await directory.list().toList(), isEmpty);

    final legacy = File('${directory.path}/legacy.json');
    await legacy.writeAsBytes(payload);
    await secure.write(key, jsonEncode({
      'path': legacy.path,
      'owner_user_id': 'owner-1',
      'content_hash': sha256.convert(payload).toString(),
      'expires_at': DateTime.now().toUtc().add(const Duration(days: 2)).toIso8601String(),
    }));
    expect(await store.load(translationID: 'fixture', bookID: 'Gen', ownerID: 'owner-1'), isNull);
    expect(await legacy.exists(), isFalse);
    expect(await secure.read(key), isNull);
  });
}

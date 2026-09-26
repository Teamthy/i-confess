import 'api_client.dart';

/// Provider-neutral Bible DTOs matching the iCONFESS `/v1/bible` contract.
final class BibleTranslation {
  const BibleTranslation({
    required this.id,
    required this.name,
    required this.abbreviation,
    required this.languageCode,
    required this.languageName,
    required this.direction,
    required this.copyAllowed,
    required this.attributionRequired,
    required this.attributionText,
  });

  final String id;
  final String name;
  final String abbreviation;
  final String languageCode;
  final String languageName;
  final String direction;
  final bool copyAllowed;
  final bool attributionRequired;
  final String attributionText;

  factory BibleTranslation.fromJson(Map<String, dynamic> json) => BibleTranslation(
        id: json['id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        abbreviation: json['abbreviation'] as String? ?? '',
        languageCode: json['language_code'] as String? ?? '',
        languageName: json['language_name'] as String? ?? '',
        direction: json['direction'] as String? ?? 'ltr',
        copyAllowed: json['copy_allowed'] as bool? ?? false,
        attributionRequired: json['attribution_required'] as bool? ?? false,
        attributionText: json['attribution_text'] as String? ?? '',
      );
}

final class BibleBook {
  const BibleBook({
    required this.id,
    required this.name,
    required this.testament,
    required this.chapterCount,
  });

  final String id;
  final String name;
  final String testament;
  final int chapterCount;

  factory BibleBook.fromJson(Map<String, dynamic> json) => BibleBook(
        id: json['id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        testament: json['testament'] as String? ?? '',
        chapterCount: (json['chapter_count'] as num?)?.toInt() ?? 0,
      );
}

final class BibleVerse {
  const BibleVerse({
    required this.id,
    required this.number,
    required this.text,
    required this.bookId,
    required this.chapter,
  });

  final String id;
  final int number;
  final String text;
  final String bookId;
  final int chapter;

  factory BibleVerse.fromJson(Map<String, dynamic> json) => BibleVerse(
        id: json['id'] as String? ?? '',
        number: (json['number'] as num?)?.toInt() ?? 0,
        text: json['text'] as String? ?? '',
        bookId: json['book_id'] as String? ?? '',
        chapter: (json['chapter'] as num?)?.toInt() ?? 0,
      );
}

final class BibleChapter {
  const BibleChapter({
    required this.translation,
    required this.book,
    required this.chapter,
    required this.verses,
  });

  final BibleTranslation translation;
  final BibleBook book;
  final int chapter;
  final List<BibleVerse> verses;

  factory BibleChapter.fromJson(Map<String, dynamic> json) => BibleChapter(
        translation: BibleTranslation.fromJson(
          Map<String, dynamic>.from(json['translation'] as Map? ?? const {}),
        ),
        book: BibleBook.fromJson(
          Map<String, dynamic>.from(json['book'] as Map? ?? const {}),
        ),
        chapter: (json['chapter'] as num?)?.toInt() ?? 0,
        verses: (json['verses'] as List? ?? const [])
            .whereType<Map<dynamic, dynamic>>()
            .map((item) => BibleVerse.fromJson(Map<String, dynamic>.from(item)))
            .toList(growable: false),
      );
}

/// API repository; provider formats are never parsed in a client.
final class BibleRepository {
  const BibleRepository(this.api);
  final ApiClient api;

  Future<List<BibleTranslation>> translations({String language = ''}) async {
    final json = await api.get('/v1/bible/translations', query: {
      if (language.isNotEmpty) 'language': language,
    });
    return (json['translations'] as List? ?? const [])
        .whereType<Map<dynamic, dynamic>>()
        .map((item) => BibleTranslation.fromJson(Map<String, dynamic>.from(item)))
        .toList(growable: false);
  }

  Future<List<BibleBook>> books(String translation) async {
    final json = await api.get('/v1/bible/books', query: {'translation': translation});
    return (json['books'] as List? ?? const [])
        .whereType<Map<dynamic, dynamic>>()
        .map((item) => BibleBook.fromJson(Map<String, dynamic>.from(item)))
        .toList(growable: false);
  }

  Future<BibleChapter> chapter(String translation, String book, int chapter) async {
    final json = await api.get('/v1/bible/$translation/$book/$chapter');
    return BibleChapter.fromJson(json);
  }

  Future<Map<String, dynamic>> search(String query, String translation) =>
      api.get('/v1/bible/search', query: {'q': query, 'translation': translation});

  Future<Map<String, dynamic>> compare(String reference, List<String> translations) =>
      api.get('/v1/bible/compare', query: {
        'reference': reference,
        'translations': translations.join(','),
      });

  Future<List<Map<String, dynamic>>> crossReferences(String reference) async =>
      _maps((await api.get('/v1/bible/cross-references', query: {'reference': reference}))['references']);

  Future<Map<String, dynamic>> verseOfDay(String translation) =>
      api.get('/v1/bible/verse-of-day', query: {'translation': translation});

  Future<List<Map<String, dynamic>>> plans() async =>
      _maps((await api.get('/v1/bible/plans'))['plans']);

  Future<Map<String, dynamic>> plan(String slug) =>
      api.get('/v1/bible/plans/$slug');

  Future<Map<String, dynamic>> enrollPlan(String planId, String translation) =>
      api.post('/v1/me/bible/plans/$planId/enroll', {'translation_id': translation});

  Future<Map<String, dynamic>> completePlanDay(String enrollmentId, int day) =>
      api.post('/v1/me/bible/plans/$enrollmentId/days/$day');

  Future<List<Map<String, dynamic>>> myPlans() async =>
      _maps((await api.get('/v1/me/bible/plans'))['plans']);

  Future<List<Map<String, dynamic>>> bookmarks() async =>
      _maps((await api.get('/v1/me/bible/bookmarks'))['bookmarks']);

  Future<Map<String, dynamic>> saveBookmark(BibleVerse verse, String translation) =>
      api.post('/v1/me/bible/bookmarks', {
        'translation_id': translation,
        'book_id': verse.bookId,
        'chapter': verse.chapter,
        'verse': verse.number,
      });

  Future<List<Map<String, dynamic>>> highlights() async =>
      _maps((await api.get('/v1/me/bible/highlights'))['highlights']);

  Future<void> deleteBookmark(String id) async {
    await api.delete('/v1/me/bible/bookmarks/$id');
  }

  Future<void> deleteHighlight(String id) async {
    await api.delete('/v1/me/bible/highlights/$id');
  }

  Future<void> deleteNote(String id) async {
    await api.delete('/v1/me/bible/notes/$id');
  }

  Future<Map<String, dynamic>> saveHighlight(
          BibleVerse verse, String translation, String color) =>
      api.post('/v1/me/bible/highlights', {
        'translation_id': translation,
        'book_id': verse.bookId,
        'chapter': verse.chapter,
        'verse': verse.number,
        'color': color,
      });

  Future<Map<String, dynamic>> saveNote({
    required BibleVerse verse,
    required String translation,
    required String body,
  }) => api.post('/v1/me/bible/notes', {
        'translation_id': translation,
        'book_id': verse.bookId,
        'chapter': verse.chapter,
        'verse_start': verse.number,
        'verse_end': verse.number,
        'body': body,
      });

  Future<List<Map<String, dynamic>>> notes() async =>
      _maps((await api.get('/v1/me/bible/notes'))['notes']);

  Future<List<Map<String, dynamic>>> collections() async =>
      _maps((await api.get('/v1/me/bible/collections'))['collections']);

  Future<Map<String, dynamic>> createCollection(String name, {String description = ''}) =>
      api.post('/v1/me/bible/collections', {'name': name, 'description': description});

  Future<Map<String, dynamic>> addVerseToCollection(String id, BibleVerse verse, String translation) =>
      api.post('/v1/me/bible/collections/$id/items', {
        'translation_id': translation,
        'book_id': verse.bookId,
        'chapter': verse.chapter,
        'verse': verse.number,
      });

  Future<void> removeCollectionItem(String collectionId, String itemId) async {
    await api.delete('/v1/me/bible/collections/$collectionId/items/$itemId');
  }

  Future<void> deleteCollection(String id) async {
    await api.delete('/v1/me/bible/collections/$id');
  }

  Future<List<Map<String, dynamic>>> history() async =>
      _maps((await api.get('/v1/me/bible/history'))['history']);

  Future<void> recordHistory({
    required String translation,
    required String book,
    required int chapter,
    required int verse,
    required String deviceId,
  }) async {
    await api.post('/v1/me/bible/history', {
      'translation_id': translation,
      'book_id': book,
      'chapter': chapter,
      'verse': verse,
      'device_id': deviceId,
    });
  }

  Future<List<Map<String, dynamic>>> progress() async =>
      _maps((await api.get('/v1/me/bible/progress'))['progress']);

  Future<void> completeChapter(String translation, String book, int chapter) async {
    await api.post('/v1/me/bible/progress', {
      'translation_id': translation,
      'book_id': book,
      'chapter': chapter,
    });
  }

  Future<Map<String, dynamic>> preferences() => api.get('/v1/me/bible/preferences');

  Future<Map<String, dynamic>> savePreferences(Map<String, dynamic> value) =>
      api.put('/v1/me/bible/preferences', value);

  Future<Map<String, dynamic>> sync(String deviceId, {String? since, List<Map<String, dynamic>> mutations = const []}) =>
      api.post('/v1/me/bible/sync', {
        'device_id': deviceId,
        if (since != null) 'since': since,
        if (mutations.isNotEmpty) 'mutations': mutations,
      });

  Future<List<Map<String, dynamic>>> offlineLicenses() async =>
      _maps((await api.get('/v1/me/bible/offline'))['licenses']);

  Future<Map<String, dynamic>> requestOfflineBook(String translation, String book) =>
      api.post('/v1/me/bible/offline', {
        'translation_id': translation,
        'book_id': book,
      });

  Future<void> revokeOfflineLicense(String id) async {
    await api.delete('/v1/me/bible/offline/$id');
  }

  Future<Map<String, dynamic>> audio(String translation, String reference) =>
      api.get('/v1/bible/audio', query: {
        'translation': translation,
        'reference': reference,
      });
}

List<Map<String, dynamic>> _maps(Object? value) =>
    (value as List? ?? const [])
        .whereType<Map<dynamic, dynamic>>()
        .map((item) => Map<String, dynamic>.from(item))
        .toList(growable: false);

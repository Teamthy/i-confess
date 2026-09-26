import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:just_audio/just_audio.dart';
import 'package:path_provider/path_provider.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

T? _firstOrNull<T>(Iterable<T> values) {
  final iterator = values.iterator;
  return iterator.moveNext() ? iterator.current : null;
}

class BibleScreen extends ConsumerStatefulWidget {
  const BibleScreen({this.initialTranslation, this.initialReference, super.key});

  final String? initialTranslation;
  final String? initialReference;

  @override
  ConsumerState<BibleScreen> createState() => _BibleScreenState();
}

class _BibleScreenState extends ConsumerState<BibleScreen> {
  List<BibleTranslation> _translations = const [];
  List<BibleBook> _books = const [];
  BibleTranslation? _translation;
  BibleChapter? _chapter;
  BibleVerse? _selectedVerse;
  bool _loading = true;
  bool _initialReferenceResolved = false;
  String? _error;
  int _chapterNumber = 1;
  double _fontSize = 21;
  double _lineHeight = 1.75;
  bool _showVerseNumbers = true;
  bool _savingStudy = false;
  String _uiLocale = 'en';
  String _defaultTranslationID = '';
  int _preferencesVersion = 0;
  String _lastSync = '';
  final AudioPlayer _audioPlayer = AudioPlayer();
  final _searchController = TextEditingController();
  static const _uiLocales = <String, String>{'en':'English','es':'Español','fr':'Français','sw':'Kiswahili','tl':'Tagalog','ar':'العربية'};
  static const _labels = <String, Map<String, String>>{
    'en': {'reading':'Scripture','translation':'Translation','book':'Book','chapter':'Chapter','search':'Search or enter a reference','copy':'Copy','bookmark':'Bookmark','highlight':'Highlight','note':'Private note','plans':'Reading plans','history':'History','preferences':'Reader preferences','verse_day':'Verse of the day','compare':'Compare translations','offline':'Download book for offline reading','complete':'Mark chapter complete','audio':'Listen to this passage','no_translations':'No reviewed translations are available.'},
    'es': {'reading':'Lectura bíblica','translation':'Traducción','book':'Libro','chapter':'Capítulo','search':'Buscar o escribir una referencia','copy':'Copiar','bookmark':'Guardar','highlight':'Resaltar','note':'Nota privada','plans':'Planes de lectura','history':'Historial','preferences':'Preferencias de lectura','verse_day':'Versículo del día','compare':'Comparar traducciones','offline':'Descargar libro para leer sin conexión','complete':'Marcar capítulo como completado','audio':'Escuchar este pasaje','no_translations':'No hay traducciones revisadas disponibles.'},
    'fr': {'reading':'Lecture biblique','translation':'Traduction','book':'Livre','chapter':'Chapitre','search':'Rechercher ou saisir une référence','copy':'Copier','bookmark':'Enregistrer','highlight':'Surligner','note':'Note privée','plans':'Plans de lecture','history':'Historique','preferences':'Préférences de lecture','verse_day':'Verset du jour','compare':'Comparer les traductions','offline':'Télécharger le livre hors ligne','complete':'Marquer le chapitre comme terminé','audio':'Écouter ce passage','no_translations':'Aucune traduction vérifiée n’est disponible.'},
    'sw': {'reading':'Usomaji wa Biblia','translation':'Tafsiri','book':'Kitabu','chapter':'Sura','search':'Tafuta au andika rejeo','copy':'Nakili','bookmark':'Hifadhi','highlight':'Angazia','note':'Dokezo la faragha','plans':'Mipango ya usomaji','history':'Historia','preferences':'Mapendeleo ya usomaji','verse_day':'Mstari wa siku','compare':'Linganisha tafsiri','offline':'Pakua kitabu kwa matumizi bila mtandao','complete':'Weka sura kuwa imesomwa','audio':'Sikiliza kifungu hiki','no_translations':'Hakuna tafsiri zilizokaguliwa zinazopatikana.'},
    'tl': {'reading':'Pagbasa ng Kasulatan','translation':'Salin','book':'Aklat','chapter':'Kabanata','search':'Maghanap o maglagay ng sanggunian','copy':'Kopyahin','bookmark':'I-save','highlight':'I-highlight','note':'Pribadong tala','plans':'Mga plano sa pagbasa','history':'Kasaysayan','preferences':'Mga kagustuhan','verse_day':'Talata ng araw','compare':'Ihambing ang mga salin','offline':'I-download ang aklat para offline','complete':'Markahang tapos ang kabanata','audio':'Pakinggan ang siping ito','no_translations':'Walang nasuring salin na magagamit.'},
    'ar': {'reading':'قراءة الكتاب المقدس','translation':'الترجمة','book':'السفر','chapter':'الإصحاح','search':'ابحث أو أدخل مرجعاً','copy':'نسخ','bookmark':'حفظ','highlight':'تمييز','note':'ملاحظة خاصة','plans':'خطط القراءة','history':'السجل','preferences':'تفضيلات القراءة','verse_day':'آية اليوم','compare':'مقارنة الترجمات','offline':'تنزيل السفر للقراءة دون اتصال','complete':'إكمال الإصحاح','audio':'استمع إلى هذا المقطع','no_translations':'لا توجد ترجمات معتمدة متاحة.'},
  };

  BibleRepository get _repository => ref.read(bibleRepositoryProvider);

  String _t(String key) => _labels[_uiLocale]?[key] ?? _labels['en']![key]!;
  TextDirection get _uiDirection => _uiLocale == 'ar' ? TextDirection.rtl : TextDirection.ltr;

  String _deviceId() {
    final prefs = ref.read(sharedPreferencesProvider);
    var id = prefs.getString('bible_sync_device_id');
    if (id == null || id.isEmpty) {
      final random = Random.secure();
      id = 'bible-${DateTime.now().microsecondsSinceEpoch}-${random.nextInt(1 << 32).toRadixString(16)}';
      prefs.setString('bible_sync_device_id', id);
    }
    return id;
  }

  String _outboxKey(String? owner) => owner == null || owner.isEmpty
      ? 'bible.sync.outbox.unbound'
      : 'bible.sync.outbox.$owner';

  Future<List<Map<String, dynamic>>> _readOutbox(String? owner) async {
    final raw = await ref.read(secureStorageProvider).read(_outboxKey(owner));
    return (jsonDecode(raw ?? '[]') as List? ?? const [])
        .whereType<Map>().map((item) => Map<String, dynamic>.from(item)).toList();
  }

  Future<String?> _refreshSyncOwner() async {
    final storage = ref.read(secureStorageProvider);
    if (!await ref.read(apiClientProvider).hasSession()) return storage.read('bible.sync.owner');
    try {
      final response = await ref.read(apiClientProvider).get('/v1/me');
      final user = Map<String, dynamic>.from(response['user'] as Map? ?? const {});
      final id = user['id'] as String?;
      if (id != null && id.isNotEmpty) await storage.write('bible.sync.owner', id);
    } catch (_) {
      // Keep the last known owner while offline; the account is verified again before sync.
    }
    return storage.read('bible.sync.owner');
  }

  @override
  void initState() {
    super.initState();
    _restorePreferences();
    _loadTranslations();
  }

  Future<void> _restorePreferences() async {
    final prefs = ref.read(sharedPreferencesProvider);
    final storedOwner = await ref.read(secureStorageProvider).read('bible.sync.owner');
    final cursorOwner = storedOwner ?? 'unbound';
    final storedCursor = prefs.getString('bible_sync_cursor_$cursorOwner') ?? '';
    if (!mounted) return;
    setState(() {
      _uiLocale = prefs.getString('bible_ui_locale') ?? 'en';
      _fontSize = prefs.getDouble('bible_font_size') ?? 21;
      _lineHeight = prefs.getDouble('bible_line_height') ?? 1.75;
      _showVerseNumbers = prefs.getBool('bible_show_verse_numbers') ?? true;
      _lastSync = storedCursor;
    });
    final api = ref.read(apiClientProvider);
    if (!await api.hasSession()) return;
    final owner = await _refreshSyncOwner();
    final cursorOwner = owner ?? 'unbound';
    final cursor = prefs.getString('bible_sync_cursor_$cursorOwner') ?? '';
    if (mounted) setState(() => _lastSync = cursor);
    try {
      final response = await _repository.preferences();
      final remote = Map<String, dynamic>.from(response['preferences'] as Map? ?? const {});
      if (!mounted) return;
      setState(() {
        _fontSize = (remote['font_size'] as num?)?.toDouble() ?? _fontSize;
        _lineHeight = (remote['line_height'] as num?)?.toDouble() ?? _lineHeight;
        _showVerseNumbers = remote['show_verse_numbers'] as bool? ?? _showVerseNumbers;
        _defaultTranslationID = remote['default_translation_id'] as String? ?? '';
        _preferencesVersion = (remote['row_version'] as num?)?.toInt() ?? 0;
      });
    } catch (_) {
      // Signed-out and offline users retain local reader preferences.
    }
  }

  @override
  void dispose() {
    _searchController.dispose();
    _audioPlayer.dispose();
    super.dispose();
  }

  Future<void> _loadTranslations() async {
    setState(() { _loading = true; _error = null; });
    try {
      final translations = await _repository.translations();
      if (!mounted) return;
      setState(() {
        _translations = translations;
        _translation = _firstOrNull(translations.where((item) => item.id.toLowerCase() == widget.initialTranslation?.toLowerCase())) ??
            _firstOrNull(translations.where((item) => item.id == _defaultTranslationID)) ??
            _firstOrNull(translations.where((item) => item.id == 'web')) ??
            (translations.isNotEmpty ? translations.first : null);
      });
      if (_translation != null) await _loadBooks(resetBook: true);
    } catch (_) {
      if (mounted) setState(() => _error = 'Bible content is temporarily unavailable. Check your connection and try again.');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _loadBooks({bool resetBook = false}) async {
    final translation = _translation;
    if (translation == null) return;
    try {
      final books = await _repository.books(translation.id);
      if (!mounted) return;
      setState(() {
        _books = books;
        if (resetBook || !books.any((book) => book.id == _chapter?.book.id)) {
          _chapterNumber = 1;
        }
      });
      if (books.isNotEmpty) {
        final openedDeepLink = await _openInitialReference(books);
        if (!openedDeepLink) {
          await _loadChapter(books.firstWhere(
            (book) => book.id == _chapter?.book.id,
            orElse: () => books.first,
          ));
        }
      }
    } catch (_) {
      if (mounted) setState(() => _error = 'The translation list could not be loaded. Please retry.');
    }
  }

  Future<bool> _openInitialReference(List<BibleBook> books) async {
    final reference = widget.initialReference;
    if (reference == null || reference.isEmpty || _initialReferenceResolved) return false;
    try {
      final data = await _repository.search(reference, _translation!.id);
      if (!mounted) return false;
      final results = (data['results'] as List? ?? const []).whereType<Map>().toList();
      if (data['kind'] == 'reference' && results.isNotEmpty) {
        final passage = Map<String, dynamic>.from(results.first);
        final verses = (passage['verses'] as List? ?? const []).whereType<Map>().toList();
        if (verses.isNotEmpty) {
          final first = Map<String, dynamic>.from(verses.first);
          final book = _firstOrNull(books.where((item) => item.id == first['book_id']));
          if (book != null) {
            _initialReferenceResolved = true;
            await _loadChapter(book, chapter: (first['chapter'] as num?)?.toInt() ?? 1);
            return true;
          }
        }
      }
      return false;
    } catch (_) {
      return false;
    } finally {
      _initialReferenceResolved = true;
    }
  }

  Future<void> _loadChapter(BibleBook? book, {int? chapter}) async {
    final translation = _translation;
    if (translation == null || book == null) return;
    setState(() { _loading = true; _error = null; _selectedVerse = null; });
    final targetChapter = chapter ?? _chapterNumber;
    try {
      final result = await _repository.chapter(translation.id, book.id, targetChapter);
      if (!mounted) return;
      setState(() { _chapter = result; _chapterNumber = targetChapter; });
      unawaited(_trackChapter(result));
    } catch (_) {
      if (!mounted) return;
      final restored = await _loadOfflineChapter(translation.id, book.id, targetChapter);
      if (!restored && mounted) setState(() => _error = 'This chapter is not available right now. Please retry.');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<bool> _loadOfflineChapter(String translationID, String bookID, int chapterNumber) async {
    try {
      final storage = ref.read(secureStorageProvider);
      final key = 'bible.offline.$translationID.$bookID';
      final raw = await storage.read(key);
      if (raw == null) return false;
      final manifest = jsonDecode(raw) as Map<String, dynamic>;
      final currentOwner = await storage.read('bible.sync.owner');
      if (currentOwner == null || manifest['owner_user_id'] != currentOwner) return false;
      final expiry = DateTime.tryParse(manifest['expires_at'] as String? ?? '');
      if (expiry == null || DateTime.now().toUtc().isAfter(expiry)) return false;
      final file = File(manifest['path'] as String);
      if (!await file.exists()) return false;
      final bytes = await file.readAsBytes();
      if (sha256.convert(bytes).toString() != manifest['content_hash']) return false;
      final payload = jsonDecode(utf8.decode(bytes)) as Map<String, dynamic>;
      final chapters = (payload['chapters'] as List? ?? const []).whereType<Map>().toList();
      for (final rawChapter in chapters) {
        final value = Map<String, dynamic>.from(rawChapter);
        if ((value['chapter'] as num?)?.toInt() == chapterNumber) {
          if (!mounted) return false;
          setState(() { _chapter = BibleChapter.fromJson(value); _chapterNumber = chapterNumber; _error = 'Offline reading · license expires ${expiry.toLocal().toString().split(' ').first}'; });
          return true;
        }
      }
    } catch (_) {
      return false;
    }
    return false;
  }

  Future<void> _saveReaderPreferences() async {
    final prefs = ref.read(sharedPreferencesProvider);
    await prefs.setString('bible_ui_locale', _uiLocale);
    await prefs.setDouble('bible_font_size', _fontSize);
    await prefs.setDouble('bible_line_height', _lineHeight);
    await prefs.setBool('bible_show_verse_numbers', _showVerseNumbers);
    final api = ref.read(apiClientProvider);
    if (!await api.hasSession()) return;
    try {
      final saved = await _repository.savePreferences({
        'default_translation_id': _translation?.id,
        'preferred_language': _translation?.languageCode,
        'font_size': _fontSize.round(),
        'line_height': _lineHeight,
        'theme': 'system',
        'show_verse_numbers': _showVerseNumbers,
        'row_version': _preferencesVersion,
      });
      _preferencesVersion = (saved['row_version'] as num?)?.toInt() ?? _preferencesVersion;
    } catch (_) {
      if (mounted) _snack('Could not sync reader preferences. They remain saved on this device.');
    }
  }

  Future<void> _trackChapter(BibleChapter chapter) async {
    final deviceID = _deviceId();
    final payload = {'translation_id': chapter.translation.id, 'book_id': chapter.book.id, 'chapter': chapter.chapter, 'verse': 1, 'device_id': deviceID};
    final api = ref.read(apiClientProvider);
    if (!await api.hasSession()) { await _queuePrivateMutation('history', payload); return; }
    try {
      await _repository.recordHistory(translation: chapter.translation.id, book: chapter.book.id, chapter: chapter.chapter, verse: 1, deviceId: deviceID);
    } on NetworkException {
      await _queuePrivateMutation('history', payload);
    } catch (_) {
      // Do not queue rejected references or server validation errors.
    }
  }

  void _snack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
  }

  Future<String> _queuePrivateMutation(String entity, Map<String, dynamic> payload) async {
    final storage = ref.read(secureStorageProvider);
    final owner = await _refreshSyncOwner();
    final key = _outboxKey(owner);
    final queue = await _readOutbox(owner);
    final random = Random.secure();
    final id = 'local-${DateTime.now().microsecondsSinceEpoch}-${random.nextInt(1 << 32).toRadixString(16)}';
    queue.add({'mutation_id': id, 'entity': entity, 'operation': 'upsert', 'payload': payload, 'row_version': 0});
    await storage.write(key, jsonEncode(queue));
    return id;
  }

  Future<void> _runPrivateAction(Future<void> Function() action, String success, {String? syncEntity, Map<String, dynamic>? syncPayload}) async {
    if (!await ref.read(apiClientProvider).hasSession()) {
      if (syncEntity != null && syncPayload != null) {
        await _queuePrivateMutation(syncEntity, syncPayload);
        _snack('Saved privately on this device; sign in to sync across devices.');
      } else {
        _snack('Sign in to save private Bible study data.');
      }
      return;
    }
    setState(() => _savingStudy = true);
    try {
      await action();
      _snack(success);
    } catch (error) {
      if (error is NetworkException && syncEntity != null && syncPayload != null) {
        await _queuePrivateMutation(syncEntity, syncPayload);
        _snack('Saved privately on this device and queued for sync.');
      } else {
        _snack('Could not save your private study item. Check your connection and retry.');
      }
    } finally {
      if (mounted) setState(() => _savingStudy = false);
    }
  }

  Future<void> _writePrivateNote(BibleVerse verse, BibleTranslation translation) async {
    final controller = TextEditingController();
    final body = await showDialog<String>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text('${_t('note')} · ${_chapter?.book.name} ${verse.chapter}:${verse.number}'),
        content: TextField(controller: controller, autofocus: true, minLines: 3, maxLines: 8, maxLength: 20000, decoration: const InputDecoration(hintText: 'Only you can see this note')),
        actions: [TextButton(onPressed: () => Navigator.pop(dialogContext), child: const Text('Cancel')), FilledButton(onPressed: () => Navigator.pop(dialogContext, controller.text.trim()), child: const Text('Save'))],
      ),
    );
    controller.dispose();
    if (body == null || body.trim().isEmpty) return;
    await _runPrivateAction(
      () async { await _repository.saveNote(verse: verse, translation: translation.id, body: body); },
      'Private note saved.',
      syncEntity: 'note',
      syncPayload: {'translation_id': translation.id, 'book_id': verse.bookId, 'chapter': verse.chapter, 'verse_start': verse.number, 'verse_end': verse.number, 'body': body},
    );
  }

  Future<void> _search() async {
    final query = _searchController.text.trim();
    final translation = _translation;
    if (query.isEmpty || translation == null) return;
    setState(() { _loading = true; _error = null; });
    try {
      final data = await _repository.search(query, translation.id);
      if (!mounted) return;
      final results = (data['results'] as List? ?? const []).whereType<Map>().toList();
      if (results.isEmpty) {
        setState(() => _error = 'No matching passages were found.');
        return;
      }
      final isReference = data['kind'] == 'reference';
      await showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        showDragHandle: true,
        builder: (context) => SafeArea(
          child: ListView(
            shrinkWrap: true,
            padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
            children: [
              Text('Search results', style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 12),
              ...results.map((raw) {
                final result = Map<String, dynamic>.from(raw);
                if (isReference) {
                  final verses = (result['verses'] as List? ?? const []).whereType<Map>().toList();
                  if (verses.isEmpty) return const SizedBox.shrink();
                  final first = Map<String, dynamic>.from(verses.first);
                  return ListTile(
                    title: Text(result['reference'] as String? ?? query),
                    subtitle: Text(verses.map((item) => item['text'] as String? ?? '').join(' '), maxLines: 3, overflow: TextOverflow.ellipsis),
                    onTap: () {
                      Navigator.pop(context);
                      final book = _firstOrNull(_books.where((item) => item.id == first['book_id']));
                      if (book != null) _loadChapter(book, chapter: (first['chapter'] as num?)?.toInt() ?? 1);
                    },
                  );
                }
                final bookRaw = Map<String, dynamic>.from(result['book'] as Map? ?? const {});
                final book = _firstOrNull(_books.where((item) => item.id == bookRaw['id']));
                return ListTile(
                  title: Text('${bookRaw['name'] ?? ''} ${result['chapter'] ?? ''}:${result['verse'] ?? ''}'),
                  subtitle: Text(result['text'] as String? ?? '', maxLines: 2, overflow: TextOverflow.ellipsis),
                  onTap: () {
                    Navigator.pop(context);
                    if (book != null) _loadChapter(book, chapter: (result['chapter'] as num?)?.toInt() ?? 1);
                  },
                );
              }),
            ],
          ),
        ),
      );
    } catch (_) {
      if (mounted) setState(() => _error = 'Search is temporarily unavailable. Please try again.');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _showVerseActions(BibleVerse verse) {
    final sourceTranslation = _chapter?.translation;
    if (sourceTranslation == null) return;
    final copyAttribution = sourceTranslation.attributionRequired && sourceTranslation.attributionText.isNotEmpty ? '\n${sourceTranslation.attributionText}' : '';
    setState(() => _selectedVerse = verse);
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      builder: (context) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 6, 20, 24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('${_chapter?.book.name ?? ''} ${verse.chapter}:${verse.number}', style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 10),
              Text(verse.text, style: Theme.of(context).textTheme.bodyLarge),
              const SizedBox(height: 16),
              Wrap(spacing: 8, children: [
                if (sourceTranslation.copyAllowed)
                  OutlinedButton.icon(
                    onPressed: () async {
                      await Clipboard.setData(ClipboardData(text: '${verse.text} — ${_chapter?.book.name} ${verse.chapter}:${verse.number} (${sourceTranslation.abbreviation})$copyAttribution'));
                      if (context.mounted) Navigator.pop(context);
                    },
                    icon: const Icon(Icons.copy_outlined), label: const Text('Copy'),
                  ),
                if (!sourceTranslation.copyAllowed)
                  const Chip(label: Text('Copy unavailable for this translation')),
                OutlinedButton.icon(onPressed: () { Navigator.pop(context); _showCrossReferences(verse); }, icon: const Icon(Icons.link), label: const Text('Cross-references')),
                OutlinedButton.icon(onPressed: () { Navigator.pop(context); _showCollections(verse); }, icon: const Icon(Icons.folder_outlined), label: const Text('Add to collection')),
                FilledButton.tonalIcon(
                  onPressed: _savingStudy || _translation == null ? null : () => _runPrivateAction(() async { await _repository.saveBookmark(verse, sourceTranslation.id); }, '${_t('bookmark')} saved privately.', syncEntity: 'bookmark', syncPayload: {'translation_id': sourceTranslation.id, 'book_id': verse.bookId, 'chapter': verse.chapter, 'verse': verse.number}),
                  icon: const Icon(Icons.bookmark_add_outlined), label: Text(_t('bookmark')),
                ),
                PopupMenuButton<String>(
                  tooltip: _t('highlight'),
                  onSelected: (color) => _runPrivateAction(() async { await _repository.saveHighlight(verse, sourceTranslation.id, color); }, '${_t('highlight')} saved privately.', syncEntity: 'highlight', syncPayload: {'translation_id': sourceTranslation.id, 'book_id': verse.bookId, 'chapter': verse.chapter, 'verse': verse.number, 'color': color}),
                  itemBuilder: (context) => const [PopupMenuItem(value: 'yellow', child: Text('Yellow')), PopupMenuItem(value: 'green', child: Text('Green')), PopupMenuItem(value: 'blue', child: Text('Blue')), PopupMenuItem(value: 'pink', child: Text('Pink'))],
                  child: Chip(avatar: const Icon(Icons.highlight_outlined, size: 18), label: Text(_t('highlight'))),
                ),
                OutlinedButton.icon(onPressed: _savingStudy ? null : () => _writePrivateNote(verse, sourceTranslation), icon: const Icon(Icons.note_add_outlined), label: Text(_t('note'))),
                IconButton(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.close), tooltip: 'Close'),
              ]),
            ],
          ),
        ),
      ),
    ).whenComplete(() { if (mounted) setState(() => _selectedVerse = null); });
  }

  Future<void> _downloadCurrentBook() async {
    final chapter = _chapter;
    if (chapter == null) return;
    if (!await ref.read(apiClientProvider).hasSession()) { _snack('Sign in to request a personal offline license.'); return; }
    try {
      final owner = await _refreshSyncOwner();
      if (owner == null) { _snack('Verify your account before downloading an offline license.'); return; }
      final manifest = await _repository.requestOfflineBook(chapter.translation.id, chapter.book.id);
      final url = Uri.parse(manifest['download_url'] as String);
      final client = HttpClient();
      try {
        final request = await client.getUrl(url);
        final response = await request.close();
        if (response.statusCode != HttpStatus.ok) throw const HttpException('Download unavailable');
        final builder = BytesBuilder(copy: false);
        await for (final chunk in response) {
          builder.add(chunk);
          if (builder.length > 32 * 1024 * 1024) throw const HttpException('Offline package too large');
        }
        final bytes = builder.takeBytes();
        final hash = sha256.convert(bytes).toString();
        if (hash != manifest['content_hash']) throw const HttpException('Package integrity check failed');
        final root = await getApplicationDocumentsDirectory();
        final directory = Directory('${root.path}/bible-offline');
        await directory.create(recursive: true);
        final file = File('${directory.path}/${chapter.translation.id}_${chapter.book.id}_$hash.json');
        await file.writeAsBytes(bytes, flush: true);
        final prefs = ref.read(sharedPreferencesProvider);
        final metadataKey = 'bible.offline.${chapter.translation.id}.${chapter.book.id}';
        final secureStorage = ref.read(secureStorageProvider);
        final previousRaw = await secureStorage.read(metadataKey);
        if (previousRaw != null) {
          try {
            final previous = jsonDecode(previousRaw) as Map<String, dynamic>;
            final previousPath = previous['path'] as String?;
            if (previousPath != null && previousPath != file.path) { final previousFile = File(previousPath); if (await previousFile.exists()) await previousFile.delete(); }
          } catch (_) { }
        }
        await secureStorage.write(metadataKey, jsonEncode({
          'path': file.path,
          'content_hash': hash,
          'expires_at': manifest['offline_expires_at'],
          'license_id': manifest['license_id'],
          'owner_user_id': owner,
          'attribution_required': manifest['attribution_required'],
          'attribution_text': manifest['attribution_text'],
        }));
        final offlineIndex = prefs.getStringList('bible_offline_index') ?? <String>[];
        if (!offlineIndex.contains(metadataKey)) offlineIndex.add(metadataKey);
        await prefs.setStringList('bible_offline_index', offlineIndex);
      } finally {
        client.close(force: true);
      }
      _snack('Offline book downloaded and integrity-checked.');
    } catch (_) {
      _snack('The offline book could not be downloaded. Check rights, storage, and connection.');
    }
  }

  Future<void> _playCurrentPassage() async {
    final chapter = _chapter;
    if (chapter == null) return;
    try {
      final response = await _repository.audio(chapter.translation.id, '${chapter.book.name} ${chapter.chapter}');
      final url = response['audio_url'] as String?;
      if (url == null || url.isEmpty) { _snack('No rights-cleared audio is available for this passage.'); return; }
      await _audioPlayer.setUrl(url);
      await _audioPlayer.play();
      if (!mounted) return;
      await showModalBottomSheet<void>(
        context: context,
        showDragHandle: true,
        builder: (sheetContext) => SafeArea(child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Text('${chapter.book.name} ${chapter.chapter} · ${chapter.translation.abbreviation}', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 12),
            StreamBuilder<PlayerState>(stream: _audioPlayer.playerStateStream, builder: (context, snapshot) {
              final playing = snapshot.data?.playing ?? false;
              return Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                IconButton(onPressed: () => _audioPlayer.seek(Duration.zero), icon: const Icon(Icons.replay_10)),
                IconButton(onPressed: () => playing ? _audioPlayer.pause() : _audioPlayer.play(), iconSize: 48, icon: Icon(playing ? Icons.pause_circle : Icons.play_circle)),
                IconButton(onPressed: () => _audioPlayer.seek(_audioPlayer.position + const Duration(seconds: 10)), icon: const Icon(Icons.forward_10)),
              ]);
            }),
            Text('Audio links are short-lived and rechecked against current translation and voice rights.'),
          ]),
        )),
      );
    } catch (_) {
      _snack('Audio is unavailable or not licensed for this translation.');
    } finally {
      await _audioPlayer.stop();
    }
  }

  Future<void> _showCrossReferences(BibleVerse verse) async {
    try {
      final reference = '${verse.bookId}.${verse.chapter}.${verse.number}';
      final references = await _repository.crossReferences(reference);
      if (!mounted) return;
      await showModalBottomSheet<void>(context: context, showDragHandle: true, builder: (sheetContext) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(sheetContext).height * .55, child: references.isEmpty ? const Center(child: Text('No reviewed cross-references for this verse.')) : ListView(children: [
        Padding(padding: const EdgeInsets.all(16), child: Text('Cross-references · $reference', style: Theme.of(sheetContext).textTheme.titleLarge)),
        ...references.map((item) => ListTile(leading: const Icon(Icons.link), title: Text(item['target_reference'] as String? ?? ''), subtitle: Text(item['editor_note'] as String? ?? ''), onTap: () { final target = item['target_reference'] as String?; if (target != null) { Navigator.pop(sheetContext); _searchController.text = target; _search(); } })),
      ]))));
    } catch (_) { _snack('Reviewed cross-references are temporarily unavailable.'); }
  }

  Future<void> _showComparison() async {
    final chapter = _chapter;
    if (chapter == null || _translations.length < 2) return;
    final second = await showModalBottomSheet<BibleTranslation>(
      context: context,
      showDragHandle: true,
      builder: (context) => SafeArea(child: ListView(
        shrinkWrap: true,
        children: [
          Padding(padding: const EdgeInsets.all(16), child: Text(_t('compare'), style: Theme.of(context).textTheme.titleLarge)),
          ..._translations.where((item) => item.id != chapter.translation.id).map((item) => ListTile(title: Text('${item.abbreviation} · ${item.name}'), onTap: () => Navigator.pop(context, item))),
        ],
      )),
    );
    if (second == null) return;
    try {
      final data = await _repository.compare('${chapter.book.name} ${chapter.chapter}', [chapter.translation.id, second.id]);
      final passages = (data['passages'] as List? ?? const []).whereType<Map>().map((item) => Map<String, dynamic>.from(item)).toList();
      if (!mounted) return;
      await showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        showDragHandle: true,
        builder: (context) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(context).height * .78, child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Text('${_t('compare')} · ${chapter.book.name} ${chapter.chapter}', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 12),
            ...passages.map((passage) {
              final translation = Map<String, dynamic>.from(passage['translation'] as Map? ?? const {});
              final verses = (passage['verses'] as List? ?? const []).whereType<Map>().map((v) => Map<String, dynamic>.from(v)).toList();
              return Card(child: Padding(padding: const EdgeInsets.all(14), child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('${translation['abbreviation'] ?? ''} · ${translation['name'] ?? ''}', style: Theme.of(context).textTheme.titleMedium),
                for (final verse in verses) Padding(padding: const EdgeInsets.only(top: 8), child: Text('${verse['number']}. ${verse['text']}', textDirection: translation['direction'] == 'rtl' ? TextDirection.rtl : TextDirection.ltr)),
                if (translation['attribution_required'] == true) Text(translation['attribution_text'] as String? ?? ''),
              ])));
            }),
          ],
        ))),
      );
    } catch (_) { _snack('This passage cannot be compared in the selected translations.'); }
  }

  Future<void> _showVerseOfDay() async {
    final translation = _chapter?.translation ?? _translation;
    if (translation == null) return;
    try {
      final result = await _repository.verseOfDay(translation.id);
      final verse = Map<String, dynamic>.from(result['verse'] as Map? ?? const {});
      final ref = result['reference'] as String? ?? '';
      if (!mounted) return;
      await showDialog<void>(context: context, builder: (context) => AlertDialog(
        title: Text(_t('verse_day')),
        content: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [Text(ref, style: Theme.of(context).textTheme.titleMedium), const SizedBox(height: 8), Text(verse['text'] as String? ?? '', textDirection: translation.direction == 'rtl' ? TextDirection.rtl : TextDirection.ltr), const SizedBox(height: 8), Text(translation.attributionText, style: Theme.of(context).textTheme.bodySmall)]),
        actions: [TextButton(onPressed: () => Navigator.pop(context), child: const Text('Close')), FilledButton(onPressed: () { Navigator.pop(context); _openStudyReference(ref); }, child: const Text('Read passage'))],
      ));
    } catch (_) { _snack('The reviewed verse of the day is temporarily unavailable.'); }
  }

  Future<void> _openStudyReference(String reference) async {
    final translation = _translation;
    if (translation == null) { _snack('Choose a reviewed translation first.'); return; }
    try {
      final data = await _repository.search(reference, translation.id);
      final passages = (data['results'] as List? ?? const []).whereType<Map>().toList();
      if (passages.isEmpty) { _snack('This canonical reading reference is unavailable in the selected translation.'); return; }
      final verses = (passages.first['verses'] as List? ?? const []).whereType<Map>().toList();
      if (verses.isEmpty) { _snack('This passage could not be opened.'); return; }
      final first = Map<String, dynamic>.from(verses.first);
      final book = _firstOrNull(_books.where((item) => item.id == first['book_id']));
      if (book == null) { _snack('The referenced book is not available in the selected translation.'); return; }
      await _loadChapter(book, chapter: (first['chapter'] as num?)?.toInt() ?? 1);
    } catch (_) { _snack('This reading reference is temporarily unavailable.'); }
  }

  Future<void> _showPlans() async {
    try {
      final plans = await _repository.plans();
      var enrolledPlans = <Map<String, dynamic>>[];
      if (await ref.read(apiClientProvider).hasSession()) {
        try { enrolledPlans = await _repository.myPlans(); } catch (_) { enrolledPlans = []; }
      }
      if (!mounted) return;
      await showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        showDragHandle: true,
        builder: (sheetContext) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(sheetContext).height * .75, child: ListView(
          padding: const EdgeInsets.all(16),
          children: [Text(_t('plans'), style: Theme.of(sheetContext).textTheme.titleLarge), const SizedBox(height: 8),
            ...plans.map((plan) => Card(child: ListTile(
              title: Text(plan['title'] as String? ?? ''),
              subtitle: Text('${plan['duration_days']} days · ${plan['description'] ?? ''}'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () async {
                final slug = plan['slug'] as String? ?? '';
                try {
                  final details = await _repository.plan(slug);
                  final body = Map<String, dynamic>.from(details['plan'] as Map? ?? const {});
                  final enrollment = _firstOrNull(enrolledPlans.where((item) => item['plan_id'] == body['id']));
                  if (!sheetContext.mounted) return;
                  await showDialog<void>(context: context, builder: (dialogContext) => AlertDialog(
                    title: Text(body['title'] as String? ?? ''),
                    content: SizedBox(width: 480, child: ListView(shrinkWrap: true, children: [Text(body['description'] as String? ?? ''), const SizedBox(height: 8), ...(body['days'] as List? ?? const []).whereType<Map>().map((day) => ListTile(dense: true, title: Text('Day ${day['day_number']}: ${day['title']}'), subtitle: Text((day['references'] as List? ?? const []).join(' · ')), onTap: () { final refs = day['references'] as List? ?? const []; if (refs.isEmpty) return; Navigator.pop(dialogContext); Navigator.pop(sheetContext); _openStudyReference(refs.first.toString()); }))])),
                    actions: [
                      TextButton(onPressed: () => Navigator.pop(dialogContext), child: const Text('Close')),
                      FilledButton(onPressed: () async {
                        if (!await ref.read(apiClientProvider).hasSession()) { _snack('Sign in to enroll and sync plan progress.'); return; }
                        try {
                          if (enrollment == null) {
                            if (_translation == null) { _snack('Choose an available translation first.'); return; }
                            await _repository.enrollPlan(body['id'] as String, _translation!.id);
                            if (dialogContext.mounted) Navigator.pop(dialogContext);
                            _snack('Reading plan started.');
                          } else {
                            final day = (enrollment['current_day'] as num?)?.toInt() ?? 1;
                            await _repository.completePlanDay(enrollment['id'] as String, day);
                            if (dialogContext.mounted) Navigator.pop(dialogContext);
                            _snack('Reading plan day $day completed.');
                          }
                        } catch (_) { _snack('Could not update this reading plan.'); }
                      }, child: Text(enrollment == null ? 'Start plan' : 'Complete day ${(enrollment['current_day'] as num?)?.toInt() ?? 1}')),
                    ],
                  ));
                } catch (_) { _snack('This reading plan is temporarily unavailable.'); }
              },
            ))),
          ],
        ))),
      );
    } catch (_) { _snack('Reading plans are temporarily unavailable.'); }
  }

  Future<void> _showCollections([BibleVerse? verse]) async {
    if (!await ref.read(apiClientProvider).hasSession()) { _snack('Sign in to sync private Bible collections.'); return; }
    try {
      var collections = await _repository.collections();
      if (!mounted) return;
      await showModalBottomSheet<void>(context: context, isScrollControlled: true, showDragHandle: true, builder: (sheetContext) => StatefulBuilder(builder: (context, setSheetState) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(context).height * .72, child: ListView(children: [
        Padding(padding: const EdgeInsets.all(16), child: Text(verse == null ? 'Private Bible collections' : 'Add ${_chapter?.book.name} ${verse.chapter}:${verse.number} to a collection', style: Theme.of(context).textTheme.titleLarge)),
        ListTile(leading: const Icon(Icons.create_new_folder_outlined), title: const Text('Create a collection'), onTap: () async {
          final name = TextEditingController();
          final value = await showDialog<String>(context: context, builder: (dialogContext) => AlertDialog(title: const Text('New private collection'), content: TextField(controller: name, autofocus: true, maxLength: 80, decoration: const InputDecoration(labelText: 'Name')), actions: [TextButton(onPressed: () => Navigator.pop(dialogContext), child: const Text('Cancel')), FilledButton(onPressed: () => Navigator.pop(dialogContext, name.text.trim()), child: const Text('Create'))]));
          name.dispose();
          if (value == null || value.isEmpty) return;
          try { await _repository.createCollection(value); collections = await _repository.collections(); setSheetState(() {}); }
          catch (_) { _snack('Could not create this collection.'); }
        }),
        if (collections.isEmpty) const ListTile(title: Text('Create a collection to keep canonical references together.')),
        ...collections.map((collection) => ListTile(leading: const Icon(Icons.folder_outlined), title: Text(collection['name'] as String? ?? ''), subtitle: Text('${(collection['items'] as List? ?? const []).length} saved reference(s)'), trailing: verse != null ? IconButton(tooltip: 'Add verse to collection', icon: const Icon(Icons.add_circle_outline), onPressed: () async { try { await _repository.addVerseToCollection(collection['id'] as String, verse, _chapter!.translation.id); if (sheetContext.mounted) Navigator.pop(sheetContext); _snack('Canonical reference added to your private collection.'); } catch (_) { _snack('Could not add this reference.'); } }) : IconButton(tooltip: 'Delete collection', icon: const Icon(Icons.delete_outline), onPressed: () async { try { await _repository.deleteCollection(collection['id'] as String); collections = await _repository.collections(); setSheetState(() {}); } catch (_) { _snack('Could not delete this collection.'); } }))),
        for (final collection in collections) if (verse == null) for (final item in (collection['items'] as List? ?? const []).whereType<Map>()) ListTile(dense: true, leading: const Icon(Icons.bookmark_outline), title: Text('${item['book_id']} ${item['chapter']}:${item['verse']}'), subtitle: Text(collection['name'] as String? ?? ''), trailing: IconButton(tooltip: 'Remove reference', icon: const Icon(Icons.remove_circle_outline), onPressed: () async { try { await _repository.removeCollectionItem(collection['id'] as String, item['id'] as String); collections = await _repository.collections(); setSheetState(() {}); } catch (_) { _snack('Could not remove this reference.'); } })),
      ])))));
    } catch (_) { _snack('Could not load private Bible collections.'); }
  }

  Future<void> _showSavedStudy() async {
    try {
      final hasSession = await ref.read(apiClientProvider).hasSession();
      final bookmarks = hasSession ? await _repository.bookmarks() : <Map<String, dynamic>>[];
      final highlights = hasSession ? await _repository.highlights() : <Map<String, dynamic>>[];
      final notes = hasSession ? await _repository.notes() : <Map<String, dynamic>>[];
      final storage = ref.read(secureStorageProvider);
      final owner = hasSession ? await _refreshSyncOwner() : await storage.read('bible.sync.owner');
      final outboxKey = _outboxKey(owner);
      final pending = await _readOutbox(owner);
      if (!mounted) return;
      await showModalBottomSheet<void>(context: context, isScrollControlled: true, showDragHandle: true, builder: (sheetContext) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(sheetContext).height * .78, child: ListView(children: [
        Padding(padding: const EdgeInsets.all(16), child: Text('Private saved passages, highlights and notes', style: Theme.of(sheetContext).textTheme.titleLarge)),
        if (bookmarks.isEmpty && highlights.isEmpty && notes.isEmpty && pending.isEmpty) const ListTile(title: Text('Nothing saved yet.')),
        if (pending.isNotEmpty) const ListTile(title: Text('Saved on this device · pending sync'), subtitle: Text('These private changes are stored in secure device storage.')),
        ...pending.map((item) {
          final payload = Map<String, dynamic>.from(item['payload'] as Map? ?? const {});
          final entity = item['entity'] as String? ?? 'study item';
          final label = entity == 'note' ? '${payload['book_id']} ${payload['chapter']}:${payload['verse_start']} · ${payload['body'] ?? ''}' : '${payload['book_id']} ${payload['chapter']}:${payload['verse']} · $entity';
          return ListTile(leading: const Icon(Icons.cloud_upload_outlined), title: Text(label, maxLines: 3, overflow: TextOverflow.ellipsis), subtitle: const Text('Only on this device until you sync'), trailing: IconButton(tooltip: 'Discard unsynced change', icon: const Icon(Icons.delete_outline), onPressed: () async { pending.remove(item); if (pending.isEmpty) { await storage.delete(outboxKey); } else { await storage.write(outboxKey, jsonEncode(pending)); } if (sheetContext.mounted) Navigator.pop(sheetContext); _snack('Unsynced private change discarded.'); }));
        }),
        ...bookmarks.map((item) => ListTile(leading: const Icon(Icons.bookmark_outline), title: Text('${item['book_id']} ${item['chapter']}:${item['verse']}'), subtitle: Text('${item['translation_id']} · Bookmark'), trailing: IconButton(tooltip: 'Delete bookmark', icon: const Icon(Icons.delete_outline), onPressed: () async { await _repository.deleteBookmark(item['id'] as String); if (sheetContext.mounted) Navigator.pop(sheetContext); _snack('Bookmark removed.'); }))),
        ...highlights.map((item) => ListTile(leading: const Icon(Icons.highlight_outlined), title: Text('${item['book_id']} ${item['chapter']}:${item['verse']}'), subtitle: Text('${item['translation_id']} · ${item['color']} highlight'), trailing: IconButton(tooltip: 'Delete highlight', icon: const Icon(Icons.delete_outline), onPressed: () async { await _repository.deleteHighlight(item['id'] as String); if (sheetContext.mounted) Navigator.pop(sheetContext); _snack('Highlight removed.'); }))),
        ...notes.map((item) => ListTile(leading: const Icon(Icons.note_outlined), title: Text('${item['book_id']} ${item['chapter']}:${item['verse_start']}'), subtitle: Text(item['body'] as String? ?? '', maxLines: 3, overflow: TextOverflow.ellipsis), trailing: IconButton(tooltip: 'Delete note', icon: const Icon(Icons.delete_outline), onPressed: () async { await _repository.deleteNote(item['id'] as String); if (sheetContext.mounted) Navigator.pop(sheetContext); _snack('Private note removed.'); }))),
      ]))));
    } catch (_) { _snack('Could not load private study data.'); }
  }

  Future<void> _showHistory() async {
    try {
      final hasSession = await ref.read(apiClientProvider).hasSession();
      final items = hasSession ? (await _repository.history()).toList() : <Map<String, dynamic>>[];
      final storage = ref.read(secureStorageProvider);
      final owner = hasSession ? await _refreshSyncOwner() : await storage.read('bible.sync.owner');
      final pending = (await _readOutbox(owner)).where((item) => item['entity'] == 'history').toList();
      for (final item in pending) {
        final payload = Map<String, dynamic>.from(item['payload'] as Map? ?? const {});
        items.insert(0, {'book_id': payload['book_id'], 'chapter': payload['chapter'], 'translation_id': payload['translation_id'], 'read_at': 'Saved on this device'});
      }
      if (!mounted) return;
      await showModalBottomSheet<void>(context: context, showDragHandle: true, builder: (sheetContext) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(sheetContext).height * .65, child: items.isEmpty ? Center(child: Text('No reading history yet.')) : ListView(children: [Padding(padding: const EdgeInsets.all(16), child: Text(_t('history'), style: Theme.of(sheetContext).textTheme.titleLarge)), ...items.map((item) => ListTile(title: Text('${item['book_id']} ${item['chapter']}'), subtitle: Text('${item['translation_id']} · ${item['read_at']}'), onTap: () { final book = _firstOrNull(_books.where((b) => b.id == item['book_id'])); if (book != null) { Navigator.pop(sheetContext); _loadChapter(book, chapter: (item['chapter'] as num).toInt()); } }))]))));
    } catch (_) { _snack('Could not load reading history.'); }
  }

  Future<void> _showProgress() async {
    try {
      final hasSession = await ref.read(apiClientProvider).hasSession();
      final completed = hasSession ? await _repository.progress() : <Map<String, dynamic>>[];
      final storage = ref.read(secureStorageProvider);
      final owner = hasSession ? await _refreshSyncOwner() : await storage.read('bible.sync.owner');
      final pending = (await _readOutbox(owner)).where((item) => item['entity'] == 'progress').toList();
      if (!mounted) return;
      await showModalBottomSheet<void>(context: context, showDragHandle: true, builder: (sheetContext) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(sheetContext).height * .65, child: ListView(children: [
        Padding(padding: const EdgeInsets.all(16), child: Text('Reading progress', style: Theme.of(sheetContext).textTheme.titleLarge)),
        if (completed.isEmpty && pending.isEmpty) const ListTile(title: Text('No completed chapters yet.')),
        ...completed.map((item) => ListTile(leading: const Icon(Icons.check_circle_outline), title: Text('${item['book_id']} ${item['chapter']}'), subtitle: Text('${item['translation_id']} · ${item['completed_at']}')),
        ...pending.map((item) { final payload = Map<String, dynamic>.from(item['payload'] as Map? ?? const {}); return ListTile(leading: const Icon(Icons.cloud_upload_outlined), title: Text('${payload['book_id']} ${payload['chapter']}'), subtitle: const Text('Completed on this device · pending sync')); }),
      ]))));
    } catch (_) { _snack('Could not load reading progress.'); }
  }

  Future<void> _syncStudyData() async {
    if (!await ref.read(apiClientProvider).hasSession()) { _snack('Sign in to sync your private Bible study.'); return; }
    try {
      final storage = ref.read(secureStorageProvider);
      final profile = await ref.read(apiClientProvider).get('/v1/me');
      final user = Map<String, dynamic>.from(profile['user'] as Map? ?? const {});
      final owner = user['id'] as String?;
      if (owner == null || owner.isEmpty) throw const HttpException('Account identity unavailable');
      await storage.write('bible.sync.owner', owner);
      final outboxKey = _outboxKey(owner);
      var pending = await _readOutbox(owner);
      final unbound = await _readOutbox(null);
      if (unbound.isNotEmpty && mounted) {
        final claim = await showDialog<bool>(context: context, builder: (dialogContext) => AlertDialog(
          title: const Text('Sync private device-only study?'),
          content: Text('There are ${unbound.length} unsynced item(s) with no account yet. Syncing will make them private to ${user['email'] ?? 'this signed-in account'}.'),
          actions: [TextButton(onPressed: () => Navigator.pop(dialogContext, false), child: const Text('Keep on device')), FilledButton(onPressed: () => Navigator.pop(dialogContext, true), child: const Text('Sync to this account'))],
        ));
        if (claim == true) {
          pending = [...pending, ...unbound];
          await storage.write(outboxKey, jsonEncode(pending));
          await storage.delete(_outboxKey(null));
        }
      }
      final prefs = ref.read(sharedPreferencesProvider);
      final since = prefs.getString('bible_sync_cursor_$owner') ?? '';
      final batch = pending.take(100).toList();
      final result = await _repository.sync(_deviceId(), since: since.isEmpty ? null : since, mutations: batch);
      final appliedIDs = (result['applied'] as List? ?? const []).whereType<Map>().map((item) => item['mutation_id'] as String?).whereType<String>().toSet();
      final conflicts = result['conflicts'] as List? ?? const [];
      final remaining = pending.where((item) => !appliedIDs.contains(item['mutation_id'])).toList();
      if (remaining.isEmpty) { await storage.delete(outboxKey); } else { await storage.write(outboxKey, jsonEncode(remaining)); }
      _lastSync = result['cursor'] as String? ?? DateTime.now().toUtc().toIso8601String();
      await prefs.setString('bible_sync_cursor_$owner', _lastSync);
      await prefs.setString('bible_sync_cursor', _lastSync);
      if (conflicts.isNotEmpty) {
        _snack('${appliedIDs.length} change(s) synced; ${conflicts.length} need review. ${remaining.length} item(s) remain safely stored on this device.');
      } else if (remaining.isNotEmpty) {
        _snack('${appliedIDs.length} offline change(s) synced; ${remaining.length} queued for the next sync.');
      } else {
        _snack(pending.isEmpty ? 'Private Bible study synced.' : '${appliedIDs.length} offline change(s) synced.');
      }
    } catch (_) { _snack('Sync paused. Your private outbox and reader settings remain on this device.'); }
  }

  Future<void> _showReaderPreferences() async {
    String locale = _uiLocale;
    double size = _fontSize;
    double height = _lineHeight;
    bool verseNumbers = _showVerseNumbers;
    await showDialog<void>(context: context, builder: (dialogContext) => StatefulBuilder(builder: (context, setDialogState) => AlertDialog(
      title: Text(_t('preferences')),
      content: Column(mainAxisSize: MainAxisSize.min, children: [
        DropdownButtonFormField<String>(value: locale, decoration: const InputDecoration(labelText: 'Interface language'), items: _uiLocales.entries.map((e) => DropdownMenuItem(value: e.key, child: Text(e.value))).toList(), onChanged: (v) => setDialogState(() { if (v != null) locale = v; })),
        Row(children: [const Text('Text size'), Expanded(child: Slider(value: size, min: 16, max: 32, divisions: 16, label: size.round().toString(), onChanged: (v) => setDialogState(() => size = v)))]),
        Row(children: [const Text('Line height'), Expanded(child: Slider(value: height, min: 1.2, max: 2.5, divisions: 13, label: height.toStringAsFixed(1), onChanged: (v) => setDialogState(() => height = v)))]),
        SwitchListTile(value: verseNumbers, onChanged: (v) => setDialogState(() => verseNumbers = v), title: const Text('Show verse numbers')),
      ]),
      actions: [TextButton(onPressed: () => Navigator.pop(dialogContext), child: const Text('Cancel')), FilledButton(onPressed: () async { setState(() { _uiLocale = locale; _fontSize = size; _lineHeight = height; _showVerseNumbers = verseNumbers; }); await _saveReaderPreferences(); if (dialogContext.mounted) Navigator.pop(dialogContext); }, child: const Text('Save'))],
    )));
  }

  Future<void> _removeLocalPackageForLicense(String licenseId) async {
    final prefs = ref.read(sharedPreferencesProvider);
    final storage = ref.read(secureStorageProvider);
    final index = prefs.getStringList('bible_offline_index') ?? <String>[];
    for (final key in List<String>.from(index)) {
      try {
        final raw = await storage.read(key);
        if (raw == null) { index.remove(key); continue; }
        final manifest = jsonDecode(raw) as Map<String, dynamic>;
        if (manifest['license_id'] != licenseId) continue;
        final path = manifest['path'] as String?;
        if (path != null) { final file = File(path); if (await file.exists()) await file.delete(); }
        await storage.delete(key);
        index.remove(key);
      } catch (_) {
        await storage.delete(key);
        index.remove(key);
      }
    }
    await prefs.setStringList('bible_offline_index', index);
  }

  Future<void> _showOfflineLicenses() async {
    if (!await ref.read(apiClientProvider).hasSession()) { _snack('Sign in to manage personal offline licenses.'); return; }
    try {
      final licenses = await _repository.offlineLicenses();
      if (!mounted) return;
      await showModalBottomSheet<void>(context: context, showDragHandle: true, builder: (sheetContext) => SafeArea(child: SizedBox(height: MediaQuery.sizeOf(sheetContext).height * .65, child: ListView(children: [Padding(padding: const EdgeInsets.all(16), child: Text(_t('offline'), style: Theme.of(sheetContext).textTheme.titleLarge)), ...licenses.map((item) => ListTile(title: Text('${item['translation_id']} · ${item['book_id']}'), subtitle: Text('Expires ${item['expires_at']}'), trailing: IconButton(tooltip: 'Revoke license', icon: const Icon(Icons.delete_outline), onPressed: () async { try { await _repository.revokeOfflineLicense(item['id'] as String); await _removeLocalPackageForLicense(item['id'] as String); if (sheetContext.mounted) Navigator.pop(sheetContext); _snack('Offline license revoked and local package removed.'); } catch (_) { _snack('Could not revoke offline license.'); } }))]))));
    } catch (_) { _snack('Could not load offline licenses.'); }
  }

  Future<void> _openStudyMenu() async {
    await showModalBottomSheet<void>(context: context, showDragHandle: true, builder: (sheetContext) => SafeArea(child: Wrap(children: [
      ListTile(leading: const Icon(Icons.calendar_today_outlined), title: Text(_t('verse_day')), onTap: () { Navigator.pop(sheetContext); _showVerseOfDay(); }),
      ListTile(leading: const Icon(Icons.menu_book_outlined), title: Text(_t('plans')), onTap: () { Navigator.pop(sheetContext); _showPlans(); }),
      ListTile(leading: const Icon(Icons.history), title: Text(_t('history')), onTap: () { Navigator.pop(sheetContext); _showHistory(); }),
      ListTile(leading: const Icon(Icons.checklist), title: const Text('Reading progress'), onTap: () { Navigator.pop(sheetContext); _showProgress(); }),
      ListTile(leading: const Icon(Icons.bookmarks_outlined), title: const Text('Saved passages, highlights and notes'), onTap: () { Navigator.pop(sheetContext); _showSavedStudy(); }),
      ListTile(leading: const Icon(Icons.folder_outlined), title: const Text('Private collections'), onTap: () { Navigator.pop(sheetContext); _showCollections(); }),
      ListTile(leading: const Icon(Icons.compare_arrows), title: Text(_t('compare')), onTap: () { Navigator.pop(sheetContext); _showComparison(); }),
      ListTile(leading: const Icon(Icons.download_outlined), title: Text(_t('offline')), onTap: () { Navigator.pop(sheetContext); _downloadCurrentBook(); }),
      ListTile(leading: const Icon(Icons.download_done_outlined), title: const Text('Manage offline licenses'), onTap: () { Navigator.pop(sheetContext); _showOfflineLicenses(); }),
      ListTile(leading: const Icon(Icons.sync), title: const Text('Sync private study'), subtitle: Text(_lastSync.isEmpty ? 'Never synced' : 'Last synced $_lastSync'), onTap: () { Navigator.pop(sheetContext); _syncStudyData(); }),
      ListTile(leading: const Icon(Icons.tune), title: Text(_t('preferences')), onTap: () { Navigator.pop(sheetContext); _showReaderPreferences(); }),
    ])));
  }

  @override
  Widget build(BuildContext context) {
    final translation = _translation;
    final chapter = _chapter;
    final textDirection = translation?.direction == 'rtl' ? TextDirection.rtl : TextDirection.ltr;
    return Directionality(
      textDirection: _uiDirection,
      child: Scaffold(
      appBar: AppBar(title: Text(_t('reading')), actions: [
        IconButton(tooltip: 'Decrease text size', onPressed: () { setState(() => _fontSize = (_fontSize - 1).clamp(16, 32).toDouble()); _saveReaderPreferences(); }, icon: const Icon(Icons.text_decrease)),
        IconButton(tooltip: 'Increase text size', onPressed: () { setState(() => _fontSize = (_fontSize + 1).clamp(16, 32).toDouble()); _saveReaderPreferences(); }, icon: const Icon(Icons.text_increase)),
        IconButton(tooltip: 'Bible study tools', onPressed: _openStudyMenu, icon: const Icon(Icons.more_vert)),
      ]),
      body: SafeArea(
        child: _translations.isEmpty && !_loading
            ? _EmptyTranslations(onRetry: _loadTranslations, message: _t('no_translations'))
            : Column(children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
                  child: Column(children: [
                    DropdownButtonFormField<BibleTranslation>(
                      value: translation,
                      decoration: InputDecoration(labelText: _t('translation'), border: const OutlineInputBorder()),
                      items: _translations.map((item) => DropdownMenuItem(value: item, child: Text('${item.abbreviation} · ${item.name}', overflow: TextOverflow.ellipsis))).toList(),
                      onChanged: (value) {
                        if (value == null) return;
                        setState(() { _translation = value; _chapter = null; _books = const []; _chapterNumber = 1; });
                        _loadBooks(resetBook: true);
                      },
                    ),
                    const SizedBox(height: 8),
                    Row(children: [
                      Expanded(child: DropdownButtonFormField<BibleBook>(
                        value: _firstOrNull(_books.where((item) => item.id == chapter?.book.id)),
                        decoration: InputDecoration(labelText: _t('book'), border: const OutlineInputBorder(), contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8)),
                        items: _books.map((item) => DropdownMenuItem(value: item, child: Text(item.name, overflow: TextOverflow.ellipsis))).toList(),
                        onChanged: (value) => _loadChapter(value, chapter: 1),
                      )),
                      const SizedBox(width: 8),
                      SizedBox(width: 122, child: DropdownButtonFormField<int>(
                        value: chapter == null ? null : chapter.chapter,
                        decoration: InputDecoration(labelText: _t('chapter'), border: const OutlineInputBorder(), contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8)),
                        items: List.generate(_firstOrNull(_books.where((item) => item.id == chapter?.book.id))?.chapterCount ?? 0, (index) => index + 1).map((value) => DropdownMenuItem(value: value, child: Text('$value'))).toList(),
                        onChanged: (value) { if (value != null) _loadChapter(chapter?.book, chapter: value); },
                      )),
                    ]),
                    const SizedBox(height: 8),
                    TextField(
                      controller: _searchController,
                      textInputAction: TextInputAction.search,
                      onSubmitted: (_) => _search(),
                      decoration: InputDecoration(
                        hintText: _t('search'),
                        prefixIcon: const Icon(Icons.search),
                        suffixIcon: IconButton(tooltip: 'Search Bible', onPressed: _search, icon: const Icon(Icons.arrow_forward)),
                        border: const OutlineInputBorder(),
                      ),
                    ),
                  ]),
                ),
                if (_error != null) Padding(padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 6), child: Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error))),
                if (_loading) const LinearProgressIndicator(minHeight: 2),
                if (chapter != null) Expanded(
                  child: Directionality(
                    textDirection: textDirection,
                    child: ListView.builder(
                      key: PageStorageKey('${translation?.id}:${chapter.book.id}:${chapter.chapter}'),
                      padding: const EdgeInsets.fromLTRB(20, 20, 20, 32),
                      itemCount: chapter.verses.length + 2,
                      itemBuilder: (context, index) {
                        if (index == 0) return Padding(
                          padding: const EdgeInsets.only(bottom: 18),
                          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                            Text('${chapter.book.name} ${chapter.chapter}', style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontFamily: 'serif', fontWeight: FontWeight.w400)),
                            if (chapter.translation.attributionRequired && chapter.translation.attributionText.isNotEmpty)
                              Padding(padding: const EdgeInsets.only(top: 6), child: Text(chapter.translation.attributionText, style: Theme.of(context).textTheme.bodySmall)),
                          ]),
                        );
                        if (index == chapter.verses.length + 1) return Padding(
                          padding: const EdgeInsets.only(top: 24),
                          child: Column(children: [
                            Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
                              OutlinedButton.icon(onPressed: chapter.chapter > 1 ? () => _loadChapter(chapter.book, chapter: chapter.chapter - 1) : null, icon: const Icon(Icons.chevron_left), label: const Text('Previous')),
                              Text('${chapter.book.name} ${chapter.chapter}', style: Theme.of(context).textTheme.labelMedium),
                              OutlinedButton.icon(onPressed: chapter.chapter < chapter.book.chapterCount ? () => _loadChapter(chapter.book, chapter: chapter.chapter + 1) : null, icon: const Icon(Icons.chevron_right), label: const Text('Next')),
                            ]),
                            Wrap(spacing: 8, alignment: WrapAlignment.center, children: [
                              OutlinedButton.icon(onPressed: _playCurrentPassage, icon: const Icon(Icons.headphones), label: Text(_t('audio'))),
                              OutlinedButton.icon(onPressed: () => _runPrivateAction(
                                () async { await _repository.completeChapter(chapter.translation.id, chapter.book.id, chapter.chapter); },
                                'Chapter progress saved.',
                                syncEntity: 'progress',
                                syncPayload: {'translation_id': chapter.translation.id, 'book_id': chapter.book.id, 'chapter': chapter.chapter},
                              ), icon: const Icon(Icons.check_circle_outline), label: Text(_t('complete'))),
                            ]),
                          ]),
                        );
                        final verse = chapter.verses[index - 1];
                        return Semantics(
                          label: 'Verse ${verse.number}. ${verse.text}',
                          button: true,
                          child: InkWell(
                            borderRadius: BorderRadius.circular(8),
                            onTap: () => _showVerseActions(verse),
                            child: Padding(
                              padding: const EdgeInsets.symmetric(vertical: 7, horizontal: 5),
                              child: RichText(text: TextSpan(style: TextStyle(color: Theme.of(context).colorScheme.onSurface, fontFamily: 'serif', fontSize: _fontSize, height: _lineHeight), children: [
                                if (_showVerseNumbers) TextSpan(text: '${verse.number} ', style: TextStyle(color: Theme.of(context).colorScheme.primary, fontFamily: 'sans-serif', fontSize: 12, fontWeight: FontWeight.w700)),
                                TextSpan(text: verse.text),
                              ])),
                            ),
                          ),
                        );
                      },
                    ),
                  ),
                ),
                if (chapter == null && !_loading && _error == null) const Expanded(child: Center(child: Text('Choose a translation to begin reading.'))),
              ]),
      ),
      ),
    );
  }
}

class _EmptyTranslations extends StatelessWidget {
  const _EmptyTranslations({required this.onRetry, required this.message});
  final VoidCallback onRetry;
  final String message;
  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(28),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Icon(Icons.menu_book_outlined, size: 44, color: Theme.of(context).colorScheme.primary),
            const SizedBox(height: 16),
            Text(message, style: Theme.of(context).textTheme.titleLarge, textAlign: TextAlign.center),
            const SizedBox(height: 10),
            const Text('Translations will appear after their source and usage rights have been reviewed. Unsupported translations are never substituted.', textAlign: TextAlign.center),
            const SizedBox(height: 16),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ]),
        ),
      );
}

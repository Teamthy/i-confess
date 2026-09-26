import 'package:iconfess_api/src/bible.dart';
import 'package:test/test.dart';

void main() {
  group('Bible DTO normalization', () {
    test('translation defaults to safe rights and LTR when fields are absent', () {
      final translation = BibleTranslation.fromJson({
        'id': 'swahili',
        'name': 'Swahili Bible',
      });
      expect(translation.id, 'swahili');
      expect(translation.direction, 'ltr');
      expect(translation.copyAllowed, isFalse);
    });

    test('chapter maps canonical verse identities and RTL metadata', () {
      final chapter = BibleChapter.fromJson({
        'translation': {
          'id': 'arabic-demo',
          'direction': 'rtl',
          'copy_allowed': false,
        },
        'book': {
          'id': 'John',
          'name': 'John',
          'chapter_count': 21,
        },
        'chapter': 3,
        'verses': [
          {
            'id': 'JHN.3.16',
            'number': 16,
            'text': 'For God so loved the world.',
            'book_id': 'John',
            'chapter': 3,
          },
        ],
      });
      expect(chapter.translation.direction, 'rtl');
      expect(chapter.book.chapterCount, 21);
      expect(chapter.verses.single.id, 'JHN.3.16');
    });
  });
}

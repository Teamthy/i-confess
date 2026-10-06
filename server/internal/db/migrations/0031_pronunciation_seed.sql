-- Pronunciation seed (audit VE-005, Quick Win 6).
--
-- The dictionary engine was complete and correct before this file existed -
-- locale-ranked lookup, aliases, per-engine overrides, the version folded into
-- the content hash - and shipped empty. A fresh deployment therefore
-- pronounced every biblical name, Nigerian place and translation acronym with
-- no guidance at all, in a product whose audience notices immediately.
--
-- What is here, and what is not:
--
--   * Respellings only, no IPA. A respelling is what an engine without
--     phoneme support speaks; IPA would be ignored by those engines and is
--     better added by the voice team after listening to a real render.
--     Hyphens separate syllables (markup.go strips them before synthesis).
--   * The conventions are anglicised where the English alphabet cannot carry
--     the sound - "Ngozi" is written "en-GOH-zee", not because that is the
--     Igbo pronunciation but because it is the closest an English voice
--     model gets without phoneme support.
--   * These are starting points for review, not a linguist's transcription.
--     Every row can be superseded at runtime through the admin upsert, which
--     bumps updated_at and therefore the dictionary version, so a correction
--     re-renders affected audio instead of leaving stale clips behind.
--
-- en-NG rows are duplicated explicitly for en-NG-PIDGIN. The lookup rule
-- deliberately does not let one variety inherit the other's entries
-- (markup.go isPidgin), so a Pidgin voice needs its own copy - and once it has
-- one, the two can be edited independently.
--
-- ON CONFLICT DO NOTHING: an entry an admin has already curated wins over the
-- seed, and re-running the migration is harmless.

INSERT INTO pronunciation_dictionary (id, term, locale, respelling, ipa, aliases, provider_overrides, category, created_at, updated_at) VALUES
  ('seed-pron-0001', 'Habakkuk', 'en-NG', 'hah-BAK-uk', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0002', 'Nehemiah', 'en-NG', 'nee-heh-MY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0003', 'Hezekiah', 'en-NG', 'hez-eh-KY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0004', 'Zedekiah', 'en-NG', 'zed-eh-KY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0005', 'Jehoshaphat', 'en-NG', 'jeh-HOSH-ah-fat', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0006', 'Nebuchadnezzar', 'en-NG', 'neb-uh-kud-NEZ-er', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0007', 'Methuselah', 'en-NG', 'meh-THOO-zeh-lah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0008', 'Melchizedek', 'en-NG', 'mel-KIZ-eh-dek', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0009', 'Zerubbabel', 'en-NG', 'zeh-RUB-eh-bel', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0010', 'Abednego', 'en-NG', 'eh-BED-nee-goh', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0011', 'Meshach', 'en-NG', 'MEE-shak', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0012', 'Shadrach', 'en-NG', 'SHAD-rak', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0013', 'Bathsheba', 'en-NG', 'bath-SHEE-bah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0014', 'Onesiphorus', 'en-NG', 'on-ee-SIF-oh-rus', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0015', 'Epaphroditus', 'en-NG', 'ee-paf-roh-DY-tus', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0016', 'Theophilus', 'en-NG', 'thee-OF-il-us', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0017', 'Barabbas', 'en-NG', 'bah-RAB-as', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0018', 'Caiaphas', 'en-NG', 'KY-ah-fas', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0019', 'Gethsemane', 'en-NG', 'geth-SEM-ah-nee', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0020', 'Golgotha', 'en-NG', 'GOL-guh-thah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0021', 'Nazareth', 'en-NG', 'NAZ-ah-reth', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0022', 'Bethlehem', 'en-NG', 'BETH-lee-hem', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0023', 'Galilee', 'en-NG', 'GAL-ah-lee', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0024', 'Samaria', 'en-NG', 'sah-MAIR-ee-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0025', 'Jericho', 'en-NG', 'JER-ih-koh', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0026', 'Bethesda', 'en-NG', 'beh-THEZ-dah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0027', 'Emmaus', 'en-NG', 'eh-MAY-us', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0028', 'Philippi', 'en-NG', 'FIL-ih-py', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0029', 'Thessalonica', 'en-NG', 'thes-ah-loh-NY-kah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0030', 'Ephesus', 'en-NG', 'EF-eh-sus', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0031', 'Colossae', 'en-NG', 'koh-LOS-ee', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0032', 'Galatia', 'en-NG', 'gah-LAY-shah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0033', 'Antioch', 'en-NG', 'AN-tee-ok', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0034', 'Damascus', 'en-NG', 'dah-MAS-kus', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0035', 'Nineveh', 'en-NG', 'NIN-eh-veh', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0036', 'Zephaniah', 'en-NG', 'zef-eh-NY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0037', 'Zechariah', 'en-NG', 'zek-eh-RY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0038', 'Haggai', 'en-NG', 'HAG-eye', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0039', 'Malachi', 'en-NG', 'MAL-ah-ky', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0040', 'Isaiah', 'en-NG', 'eye-ZAY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0041', 'Jeremiah', 'en-NG', 'jer-eh-MY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0042', 'Ezekiel', 'en-NG', 'eh-ZEE-kee-el', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0043', 'Obadiah', 'en-NG', 'oh-bah-DY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0044', 'Josiah', 'en-NG', 'joh-SY-ah', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0045', 'Ephraim', 'en-NG', 'EE-fray-im', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0046', 'Manasseh', 'en-NG', 'mah-NAS-eh', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0047', 'Sinai', 'en-NG', 'SY-ny', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0048', 'Zion', 'en-NG', 'ZY-un', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0049', 'Bethel', 'en-NG', 'BETH-el', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0050', 'Shiloh', 'en-NG', 'SHY-loh', NULL, '', '{}', 'biblical_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0051', 'Deuteronomy', 'en-NG', 'doo-teh-RON-eh-mee', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0052', 'Ecclesiastes', 'en-NG', 'ih-klee-zee-AS-teez', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0053', 'Leviticus', 'en-NG', 'leh-VIT-ih-kus', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0054', 'Lamentations', 'en-NG', 'lam-en-TAY-shunz', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0055', 'Ephesians', 'en-NG', 'eh-FEE-zhunz', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0056', 'Philippians', 'en-NG', 'fih-LIP-ee-unz', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0057', 'Colossians', 'en-NG', 'koh-LOSH-unz', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0058', 'Thessalonians', 'en-NG', 'thes-ah-LOH-nee-unz', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0059', 'Philemon', 'en-NG', 'fih-LEE-mun', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0060', 'Hebrews', 'en-NG', 'HEE-brooz', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0061', 'Nahum', 'en-NG', 'NAY-hum', NULL, '', '{}', 'bible_book', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0062', 'Hallelujah', 'en-NG', 'hal-eh-LOO-yah', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0063', 'Hosanna', 'en-NG', 'hoh-ZAN-ah', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0064', 'Selah', 'en-NG', 'SEE-lah', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0065', 'Immanuel', 'en-NG', 'ih-MAN-yoo-el', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0066', 'Pentecost', 'en-NG', 'PEN-teh-kost', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0067', 'Ekklesia', 'en-NG', 'eh-KLAY-see-ah', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0068', 'Sanctifier', 'en-NG', 'SANK-tih-fy-er', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0069', 'Intercessor', 'en-NG', 'in-ter-SES-or', NULL, '', '{}', 'church_term', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0070', 'KJV', 'en-NG', 'K J V', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0071', 'NKJV', 'en-NG', 'N K J V', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0072', 'NIV', 'en-NG', 'N I V', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0073', 'ESV', 'en-NG', 'E S V', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0074', 'NLT', 'en-NG', 'N L T', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0075', 'AMP', 'en-NG', 'A M P', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0076', 'MSG', 'en-NG', 'M S G', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0077', 'GNB', 'en-NG', 'G N B', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0078', 'ASV', 'en-NG', 'A S V', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0079', 'RSV', 'en-NG', 'R S V', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0080', 'YLT', 'en-NG', 'Y L T', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0081', 'TPT', 'en-NG', 'T P T', NULL, '', '{}', 'bible_translation', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0082', 'Chukwuemeka', 'en-NG', 'choo-kwoo-eh-MEH-kah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0083', 'Chidinma', 'en-NG', 'chee-DEEN-mah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0084', 'Emeka', 'en-NG', 'eh-MEH-kah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0085', 'Obinna', 'en-NG', 'oh-BEE-nah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0086', 'Ifeoma', 'en-NG', 'ee-feh-OH-mah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0087', 'Nneka', 'en-NG', 'en-NEH-kah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0088', 'Chiamaka', 'en-NG', 'chee-ah-MAH-kah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0089', 'Ndidi', 'en-NG', 'en-DEE-dee', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0090', 'Kelechi', 'en-NG', 'keh-LEH-chee', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0091', 'Uche', 'en-NG', 'oo-CHEH', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0092', 'Chinedu', 'en-NG', 'chee-NEH-doo', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0093', 'Ngozi', 'en-NG', 'en-GOH-zee', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0094', 'Adaeze', 'en-NG', 'ah-DAY-zeh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0095', 'Oluwaseun', 'en-NG', 'oh-loo-wah-SEH-oon', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0096', 'Adebayo', 'en-NG', 'ah-deh-BAH-yoh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0097', 'Folake', 'en-NG', 'foh-LAH-keh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0098', 'Temitope', 'en-NG', 'teh-mee-TOH-peh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0099', 'Ayodele', 'en-NG', 'ah-yoh-DEH-leh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0100', 'Olumide', 'en-NG', 'oh-loo-MEE-deh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0101', 'Yemi', 'en-NG', 'YEH-mee', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0102', 'Babatunde', 'en-NG', 'bah-bah-TOON-deh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0103', 'Tunde', 'en-NG', 'TOON-deh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0104', 'Sade', 'en-NG', 'SHAH-deh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0105', 'Bisi', 'en-NG', 'BEE-see', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0106', 'Kunle', 'en-NG', 'KOON-leh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0107', 'Funmi', 'en-NG', 'FOON-mee', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0108', 'Adewale', 'en-NG', 'ah-deh-WAH-leh', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0109', 'Oluwatobi', 'en-NG', 'oh-loo-wah-TOH-bee', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0110', 'Aminu', 'en-NG', 'ah-MEE-noo', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0111', 'Aisha', 'en-NG', 'ah-EE-shah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0112', 'Fatima', 'en-NG', 'fah-TEE-mah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0113', 'Ibrahim', 'en-NG', 'ee-brah-HEEM', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0114', 'Musa', 'en-NG', 'MOO-sah', NULL, '', '{}', 'nigerian_name', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0115', 'Onitsha', 'en-NG', 'oh-NEE-chah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0116', 'Ile-Ife', 'en-NG', 'ee-leh-EE-feh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0117', 'Abeokuta', 'en-NG', 'ah-beh-oh-KOO-tah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0118', 'Oshogbo', 'en-NG', 'oh-SHOG-boh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0119', 'Osogbo', 'en-NG', 'oh-SHOG-boh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0120', 'Ogbomosho', 'en-NG', 'og-boh-moh-SHOH', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0121', 'Lagos', 'en-NG', 'LAH-gohs', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0122', 'Abuja', 'en-NG', 'ah-BOO-jah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0123', 'Ibadan', 'en-NG', 'ee-BAH-dahn', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0124', 'Enugu', 'en-NG', 'eh-NOO-goo', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0125', 'Kano', 'en-NG', 'KAH-noh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0126', 'Jos', 'en-NG', 'JOHSS', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0127', 'Benin City', 'en-NG', 'beh-NEEN City', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0128', 'Calabar', 'en-NG', 'KAL-ah-bar', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0129', 'Owerri', 'en-NG', 'oh-WEH-ree', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0130', 'Akure', 'en-NG', 'ah-KOO-reh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0131', 'Ilorin', 'en-NG', 'ee-LOH-reen', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0132', 'Kaduna', 'en-NG', 'kah-DOO-nah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0133', 'Maiduguri', 'en-NG', 'my-doo-GOO-ree', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0134', 'Warri', 'en-NG', 'WAH-ree', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0135', 'Asaba', 'en-NG', 'ah-SAH-bah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0136', 'Uyo', 'en-NG', 'OO-yoh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0137', 'Yenagoa', 'en-NG', 'yeh-nah-GOH-ah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0138', 'Nsukka', 'en-NG', 'en-SOO-kah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0139', 'Awka', 'en-NG', 'AW-kah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0140', 'Aba', 'en-NG', 'ah-BAH', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0141', 'Umuahia', 'en-NG', 'oo-moo-AH-hee-ah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0142', 'Zaria', 'en-NG', 'ZAH-ree-ah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0143', 'Sokoto', 'en-NG', 'SOH-koh-toh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0144', 'Makurdi', 'en-NG', 'mah-KOOR-dee', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0145', 'Lokoja', 'en-NG', 'loh-KOH-jah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0146', 'Bauchi', 'en-NG', 'BOW-chee', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0147', 'Gombe', 'en-NG', 'GOM-beh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0148', 'Minna', 'en-NG', 'MEEN-nah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0149', 'Katsina', 'en-NG', 'kat-SEE-nah', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0150', 'Jalingo', 'en-NG', 'jah-LEEN-goh', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0151', 'Port Harcourt', 'en-NG', 'Port HAR-kort', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z'),
  ('seed-pron-0152', 'Ogbomoso', 'en-NG', 'og-boh-moh-SOH', NULL, '', '{}', 'nigerian_place', '2026-10-06T00:00:00Z', '2026-10-06T00:00:00Z')
ON CONFLICT (term, locale) DO NOTHING;

-- The same vocabulary for Pidgin voices, as its own rows rather than an
-- implicit inheritance.
INSERT INTO pronunciation_dictionary (id, term, locale, respelling, ipa, aliases, provider_overrides, category, created_at, updated_at)
SELECT 'seed-pcm-' || substr(id, 11), term, 'en-NG-PIDGIN', respelling, ipa, aliases, provider_overrides, category, created_at, updated_at
FROM pronunciation_dictionary
WHERE locale = 'en-NG' AND id LIKE 'seed-pron-%'
ON CONFLICT (term, locale) DO NOTHING;

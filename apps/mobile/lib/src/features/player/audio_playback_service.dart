import 'dart:async';
import 'package:audio_session/audio_session.dart';
import 'package:just_audio/just_audio.dart';

/// The status of low-level audio stream playback.
enum AudioPlaybackStatus {
  idle,
  loading,
  ready,
  playing,
  paused,
  completed,
  error,
}

/// Abstract audio playback service for playing server-issued signed audio streams.
///
/// Decouples audio player implementation (just_audio) from session lifecycle business logic.
abstract interface class AudioPlaybackService {
  Future<void> load(String url, {Duration? initialPosition});
  Future<void> play();
  Future<void> pause();
  Future<void> seek(Duration position);
  Future<void> stop();
  Future<void> dispose();

  Stream<Duration> get positionStream;
  Stream<Duration?> get durationStream;
  Stream<AudioPlaybackStatus> get statusStream;
  Stream<String> get errorStream;
  Stream<void> get interruptionStream;
  Stream<void> get becomingNoisyStream;

  AudioPlaybackStatus get currentStatus;
  Duration get currentPosition;
}

/// Production implementation using just_audio and audio_session.
class JustAudioPlaybackService implements AudioPlaybackService {
  JustAudioPlaybackService({AudioPlayer? player}) : _player = player ?? AudioPlayer() {
    _init();
  }

  final AudioPlayer _player;
  AudioPlaybackStatus _status = AudioPlaybackStatus.idle;

  final _statusController = StreamController<AudioPlaybackStatus>.broadcast();
  final _errorController = StreamController<String>.broadcast();
  final _interruptionController = StreamController<void>.broadcast();
  final _becomingNoisyController = StreamController<void>.broadcast();

  StreamSubscription<PlayerState>? _playerStateSub;
  StreamSubscription<PlaybackEvent>? _playbackEventSub;
  StreamSubscription<AudioInterruptionEvent>? _interruptionSub;
  StreamSubscription<void>? _becomingNoisySub;

  void _init() async {
    _playerStateSub = _player.playerStateStream.listen((state) {
      if (state.processingState == ProcessingState.completed) {
        _updateStatus(AudioPlaybackStatus.completed);
      } else if (state.processingState == ProcessingState.loading ||
          state.processingState == ProcessingState.buffering) {
        _updateStatus(AudioPlaybackStatus.loading);
      } else if (state.processingState == ProcessingState.ready) {
        _updateStatus(state.playing ? AudioPlaybackStatus.playing : AudioPlaybackStatus.paused);
      } else if (state.processingState == ProcessingState.idle) {
        _updateStatus(AudioPlaybackStatus.idle);
      }
    });

    _playbackEventSub = _player.playbackEventStream.listen(
      (_) {},
      onError: (Object e, StackTrace st) {
        _updateStatus(AudioPlaybackStatus.error);
        _errorController.add(e.toString());
      },
    );

    try {
      final session = await AudioSession.instance;
      await session.configure(const AudioSessionConfiguration.speech());

      _interruptionSub = session.interruptionEventStream.listen((event) {
        if (event.begin) {
          _interruptionController.add(null);
        }
      });

      _becomingNoisySub = session.becomingNoisyEventStream.listen((_) {
        _becomingNoisyController.add(null);
      });
    } catch (e) {
      // AudioSession may fail on unsupported platforms / desktop tests
    }
  }

  void _updateStatus(AudioPlaybackStatus status) {
    if (_status != status) {
      _status = status;
      _statusController.add(status);
    }
  }

  @override
  AudioPlaybackStatus get currentStatus => _status;

  @override
  Duration get currentPosition => _player.position;

  @override
  Stream<Duration> get positionStream => _player.positionStream;

  @override
  Stream<Duration?> get durationStream => _player.durationStream;

  @override
  Stream<AudioPlaybackStatus> get statusStream => _statusController.stream;

  @override
  Stream<String> get errorStream => _errorController.stream;

  @override
  Stream<void> get interruptionStream => _interruptionController.stream;

  @override
  Stream<void> get becomingNoisyStream => _becomingNoisyController.stream;

  @override
  Future<void> load(String url, {Duration? initialPosition}) async {
    _updateStatus(AudioPlaybackStatus.loading);
    try {
      await _player.setUrl(url, initialPosition: initialPosition);
      _updateStatus(AudioPlaybackStatus.ready);
    } catch (e) {
      _updateStatus(AudioPlaybackStatus.error);
      _errorController.add(e.toString());
      rethrow;
    }
  }

  @override
  Future<void> play() async {
    try {
      await _player.play();
    } catch (e) {
      _updateStatus(AudioPlaybackStatus.error);
      _errorController.add(e.toString());
      rethrow;
    }
  }

  @override
  Future<void> pause() async {
    await _player.pause();
  }

  @override
  Future<void> seek(Duration position) async {
    await _player.seek(position);
  }

  @override
  Future<void> stop() async {
    await _player.stop();
    _updateStatus(AudioPlaybackStatus.idle);
  }

  @override
  Future<void> dispose() async {
    await _playerStateSub?.cancel();
    await _playbackEventSub?.cancel();
    await _interruptionSub?.cancel();
    await _becomingNoisySub?.cancel();
    await _statusController.close();
    await _errorController.close();
    await _interruptionController.close();
    await _becomingNoisyController.close();
    await _player.dispose();
  }
}

/// In-memory fake audio playback service for widget and unit tests.
class TestAudioPlaybackService implements AudioPlaybackService {
  AudioPlaybackStatus _status = AudioPlaybackStatus.idle;
  Duration _position = Duration.zero;
  Duration? _duration = const Duration(seconds: 60);

  final _positionController = StreamController<Duration>.broadcast();
  final _durationController = StreamController<Duration?>.broadcast();
  final _statusController = StreamController<AudioPlaybackStatus>.broadcast();
  final _errorController = StreamController<String>.broadcast();
  final _interruptionController = StreamController<void>.broadcast();
  final _becomingNoisyController = StreamController<void>.broadcast();

  String? loadedUrl;
  Duration? loadedInitialPosition;
  int playCount = 0;
  int pauseCount = 0;
  int stopCount = 0;
  Duration? lastSeek;

  @override
  AudioPlaybackStatus get currentStatus => _status;

  @override
  Duration get currentPosition => _position;

  @override
  Stream<Duration> get positionStream => _positionController.stream;

  @override
  Stream<Duration?> get durationStream => _durationController.stream;

  @override
  Stream<AudioPlaybackStatus> get statusStream => _statusController.stream;

  @override
  Stream<String> get errorStream => _errorController.stream;

  @override
  Stream<void> get interruptionStream => _interruptionController.stream;

  @override
  Stream<void> get becomingNoisyStream => _becomingNoisyController.stream;

  void emitStatus(AudioPlaybackStatus status) {
    _status = status;
    _statusController.add(status);
  }

  void emitPosition(Duration position) {
    _position = position;
    _positionController.add(position);
  }

  void emitDuration(Duration? duration) {
    _duration = duration;
    _durationController.add(duration);
  }

  void emitError(String error) {
    _status = AudioPlaybackStatus.error;
    _statusController.add(AudioPlaybackStatus.error);
    _errorController.add(error);
  }

  void emitInterruption() {
    _interruptionController.add(null);
  }

  void emitBecomingNoisy() {
    _becomingNoisyController.add(null);
  }

  @override
  Future<void> load(String url, {Duration? initialPosition}) async {
    loadedUrl = url;
    loadedInitialPosition = initialPosition;
    _position = initialPosition ?? Duration.zero;
    emitStatus(AudioPlaybackStatus.ready);
  }

  @override
  Future<void> play() async {
    playCount++;
    emitStatus(AudioPlaybackStatus.playing);
  }

  @override
  Future<void> pause() async {
    pauseCount++;
    emitStatus(AudioPlaybackStatus.paused);
  }

  @override
  Future<void> seek(Duration position) async {
    lastSeek = position;
    emitPosition(position);
  }

  @override
  Future<void> stop() async {
    stopCount++;
    emitStatus(AudioPlaybackStatus.idle);
  }

  @override
  Future<void> dispose() async {
    await _positionController.close();
    await _durationController.close();
    await _statusController.close();
    await _errorController.close();
    await _interruptionController.close();
    await _becomingNoisyController.close();
  }
}

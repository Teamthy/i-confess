/// Audio feature exports.
///
/// This file exports all audio-related components for easy importing.
/// 
/// Phase 1: Core Audio Services (Backend)
/// Phase 2: Flutter Audio Player Integration
/// Phase 3: Background Playback & Queue Management
library audio;

// Models
export 'models/audio_asset.dart';
export 'models/audio_generation_job.dart';
export 'models/audio_generation_request.dart';
export 'models/audio_queue.dart';
export 'models/tts_provider.dart';

// Services
export 'services/audio_generation_service.dart';
export 'services/audio_url_service.dart';
export 'services/background_playback_service.dart';

// Controllers
export 'controllers/audio_player_controller.dart';
export 'controllers/queue_controller.dart';

// Providers
export 'providers/audio_providers.dart';

// Widgets
export 'widgets/audio_player_widget.dart';
export 'widgets/queue_panel.dart';

// Configuration
export 'services/background_playback_service.dart' show BackgroundPlaybackConfig, BackgroundPlaybackService;

// Enums for convenience
export 'models/audio_queue.dart' show RepeatMode;

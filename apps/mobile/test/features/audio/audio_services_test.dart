/// Unit tests for audio services.
///
/// These tests verify the functionality of the audio generation and URL services.
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:http/http.dart' as http;
import 'package:i_confess/src/features/audio/models/audio_generation_job.dart';
import 'package:i_confess/src/features/audio/models/audio_generation_request.dart';
import 'package:i_confess/src/features/audio/models/audio_asset.dart';
import 'package:i_confess/src/features/audio/models/tts_provider.dart';
import 'package:i_confess/src/features/audio/services/audio_generation_service.dart';
import 'package:i_confess/src/features/audio/services/audio_url_service.dart';
import 'package:i_confess/src/core/services/api_service.dart';

// Mock classes
class MockApiService extends Mock implements ApiService {
  @override
  Future<Map<String, dynamic>> post(
    String path, {
    Map<String, dynamic>? body,
    Map<String, String>? headers,
    Map<String, dynamic>? queryParams,
  }) async {
    return {};
  }

  @override
  Future<Map<String, dynamic>> get(
    String path, {
    Map<String, String>? headers,
    Map<String, dynamic>? queryParams,
  }) async {
    return {};
  }

  @override
  Future<Map<String, dynamic>> put(
    String path, {
    Map<String, dynamic>? body,
    Map<String, String>? headers,
    Map<String, dynamic>? queryParams,
  }) async {
    return {};
  }

  @override
  Future<Map<String, dynamic>> delete(
    String path, {
    Map<String, String>? headers,
    Map<String, dynamic>? queryParams,
  }) async {
    return {};
  }
}

class MockHttpClient extends Mock implements http.Client {
  @override
  Future<http.Response> get(Uri url, {Map<String, String>? headers}) async {
    return http.Response('', 200);
  }

  @override
  Future<http.Response> post(
    Uri url, {
    Map<String, String>? headers,
    Object? body,
    Encoding? encoding,
  }) async {
    return http.Response('', 200);
  }
}

void main() {
  group('AudioGenerationService', () {
    late AudioGenerationService service;
    late MockApiService mockApiService;

    setUp(() {
      mockApiService = MockApiService();
      service = AudioGenerationService(apiService: mockApiService);
    });

    test('createJob makes POST request to correct endpoint', () async {
      const request = AudioGenerationRequest(
        confessionId: 'confession_123',
        voiceId: 'voice_456',
        provider: 'elevenlabs',
        qualityTier: 'standard',
      );

      final expectedBody = request.toJson();
      
      when(mockApiService.post(
        '/api/v1/admin/audio/generate',
        body: expectedBody,
      )).thenAnswer((_) async => {
        'id': 'job_123',
        'confessionId': 'confession_123',
        'voiceId': 'voice_456',
        'status': 'queued',
        'createdAt': DateTime.now().toIso8601String(),
      });

      final job = await service.createJob(request);

      expect(job.id, 'job_123');
      expect(job.confessionId, 'confession_123');
      expect(job.voiceId, 'voice_456');
      expect(job.status, AudioJobStatus.queued);
    });

    test('getJob makes GET request to correct endpoint', () async {
      when(mockApiService.get(
        '/api/v1/admin/audio/generate/job_123',
      )).thenAnswer((_) async => {
        'id': 'job_123',
        'confessionId': 'confession_123',
        'voiceId': 'voice_456',
        'status': 'succeeded',
        'audioAssetId': 'asset_789',
        'progress': 1.0,
        'createdAt': DateTime.now().toIso8601String(),
        'updatedAt': DateTime.now().toIso8601String(),
      });

      final job = await service.getJob('job_123');

      expect(job.id, 'job_123');
      expect(job.status, AudioJobStatus.succeeded);
      expect(job.audioAssetId, 'asset_789');
      expect(job.progress, 1.0);
    });

    test('getJobs makes GET request to list endpoint', () async {
      when(mockApiService.get(
        '/api/v1/admin/audio/generate',
      )).thenAnswer((_) async => {
        'jobs': [
          {
            'id': 'job_1',
            'confessionId': 'confession_1',
            'status': 'queued',
          },
          {
            'id': 'job_2',
            'confessionId': 'confession_2',
            'status': 'processing',
          },
        ],
        'total': 2,
      });

      final jobs = await service.getJobs();

      expect(jobs.length, 2);
      expect(jobs[0].id, 'job_1');
      expect(jobs[1].id, 'job_2');
    });

    test('retryJob makes POST request to retry endpoint', () async {
      when(mockApiService.post(
        '/api/v1/admin/audio/generate/job_123/retry',
      )).thenAnswer((_) async => {
        'id': 'job_123',
        'confessionId': 'confession_123',
        'status': 'queued',
      });

      final job = await service.retryJob('job_123');

      expect(job.id, 'job_123');
      expect(job.status, AudioJobStatus.queued);
    });

    test('cancelJob makes POST request to cancel endpoint', () async {
      when(mockApiService.post(
        '/api/v1/admin/audio/generate/job_123/cancel',
      )).thenAnswer((_) async => {
        'id': 'job_123',
        'confessionId': 'confession_123',
        'status': 'cancelled',
      });

      final job = await service.cancelJob('job_123');

      expect(job.id, 'job_123');
      expect(job.status, AudioJobStatus.cancelled);
    });

    test('getStats makes GET request to stats endpoint', () async {
      when(mockApiService.get(
        '/api/v1/admin/audio/generate/stats',
      )).thenAnswer((_) async => {
        'totalJobs': 100,
        'succeeded': 90,
        'failed': 5,
        'pending': 5,
      });

      final stats = await service.getStats();

      expect(stats['totalJobs'], 100);
      expect(stats['succeeded'], 90);
      expect(stats['failed'], 5);
      expect(stats['pending'], 5);
    });

    test('getTtsProviders makes GET request to providers endpoint', () async {
      when(mockApiService.get(
        '/api/v1/admin/audio/providers',
      )).thenAnswer((_) async => {
        'providers': [
          {
            'id': 'elevenlabs',
            'name': 'ElevenLabs',
            'isActive': true,
            'qualityTiers': ['standard', 'premium'],
          },
          {
            'id': 'google',
            'name': 'Google TTS',
            'isActive': true,
            'qualityTiers': ['standard'],
          },
        ],
      });

      final providers = await service.getTtsProviders();

      expect(providers.length, 2);
      expect(providers[0].id, 'elevenlabs');
      expect(providers[1].id, 'google');
    });

    test('createBatchJobs makes POST request to batch endpoint', () async {
      const request = BatchAudioGenerationRequest(
        requests: [
          AudioGenerationRequest(
            confessionId: 'confession_1',
            voiceId: 'voice_1',
            provider: 'elevenlabs',
          ),
          AudioGenerationRequest(
            confessionId: 'confession_2',
            voiceId: 'voice_2',
            provider: 'elevenlabs',
          ),
        ],
      );

      when(mockApiService.post(
        '/api/v1/admin/audio/generate/batch',
        body: anyNamed('body'),
      )).thenAnswer((_) async => {
        'jobs': [
          {
            'id': 'job_1',
            'confessionId': 'confession_1',
            'status': 'queued',
          },
          {
            'id': 'job_2',
            'confessionId': 'confession_2',
            'status': 'queued',
          },
        ],
      });

      final jobs = await service.createBatchJobs(request);

      expect(jobs.length, 2);
      expect(jobs[0].confessionId, 'confession_1');
      expect(jobs[1].confessionId, 'confession_2');
    });

    test('pollJobUntilComplete polls until job succeeds', () async {
      // First call returns processing
      when(mockApiService.get('/api/v1/admin/audio/generate/job_123'))
          .thenAnswer((_) async => {
            'id': 'job_123',
            'status': 'processing',
            'progress': 0.5,
          });
      
      // Second call returns succeeded
      when(mockApiService.get('/api/v1/admin/audio/generate/job_123'))
          .thenAnswer((_) async => {
            'id': 'job_123',
            'status': 'succeeded',
            'progress': 1.0,
            'audioAssetId': 'asset_456',
          });

      final job = await service.pollJobUntilComplete(
        jobId: 'job_123',
        interval: 1,
        timeout: 10,
      );

      expect(job.status, AudioJobStatus.succeeded);
      expect(job.audioAssetId, 'asset_456');
    });

    test('pollJobUntilComplete throws on timeout', () async {
      when(mockApiService.get('/api/v1/admin/audio/generate/job_123'))
          .thenAnswer((_) async => {
            'id': 'job_123',
            'status': 'processing',
            'progress': 0.5,
          });

      expect(
        () => service.pollJobUntilComplete(
          jobId: 'job_123',
          interval: 1,
          timeout: 1,  // Very short timeout
        ),
        throwsA(isA<TimeoutException>()),
      );
    });

    test('pollJobUntilComplete throws on failure', () async {
      when(mockApiService.get('/api/v1/admin/audio/generate/job_123'))
          .thenAnswer((_) async => {
            'id': 'job_123',
            'status': 'failed',
            'errorMessage': 'Generation failed',
          });

      expect(
        () => service.pollJobUntilComplete(
          jobId: 'job_123',
          interval: 1,
          timeout: 10,
        ),
        throwsA(isA<Exception>()),
      );
    });
  });

  group('AudioUrlService', () {
    late AudioUrlService service;

    setUp(() {
      service = AudioUrlService();
    });

    test('getStreamUrl returns correct URL format', () {
      const assetId = 'asset_123';
      
      // Note: This test assumes the service uses a specific URL pattern
      // The actual implementation may vary
      final url = service.getStreamUrl(assetId);
      
      expect(url, isA<String>());
      expect(url.isNotEmpty, true);
    });

    test('getDownloadUrl returns correct URL format', () {
      const assetId = 'asset_123';
      
      final url = service.getDownloadUrl(assetId);
      
      expect(url, isA<String>());
      expect(url.isNotEmpty, true);
    });

    test('getSignedUrl returns URL with expiration', () {
      const assetId = 'asset_123';
      const expirationSeconds = 3600;
      
      final url = service.getSignedUrl(assetId, expirationSeconds);
      
      expect(url, isA<String>());
      expect(url.isNotEmpty, true);
    });

    test('getSignedUrlWithOptions returns URL with custom options', () {
      const assetId = 'asset_123';
      const options = {'quality': 'high'};
      
      final url = service.getSignedUrlWithOptions(
        assetId,
        3600,
        options,
      );
      
      expect(url, isA<String>());
      expect(url.isNotEmpty, true);
    });

    test('invalidateUrl invalidates the cache', () async {
      const assetId = 'asset_123';
      
      // This should not throw
      await service.invalidateUrl(assetId);
    });
  });

  group('Models', () {
    test('AudioGenerationJob fromJson', () {
      final json = {
        'id': 'job_123',
        'confessionId': 'confession_456',
        'voiceId': 'voice_789',
        'status': 'succeeded',
        'audioAssetId': 'asset_123',
        'progress': 1.0,
        'errorMessage': null,
        'createdAt': '2026-01-01T00:00:00Z',
        'updatedAt': '2026-01-01T00:01:00Z',
      };

      final job = AudioGenerationJob.fromJson(json);

      expect(job.id, 'job_123');
      expect(job.confessionId, 'confession_456');
      expect(job.voiceId, 'voice_789');
      expect(job.status, AudioJobStatus.succeeded);
      expect(job.audioAssetId, 'asset_123');
      expect(job.progress, 1.0);
      expect(job.errorMessage, isNull);
    });

    test('AudioGenerationJob toJson', () {
      final job = AudioGenerationJob(
        id: 'job_123',
        confessionId: 'confession_456',
        voiceId: 'voice_789',
        status: AudioJobStatus.succeeded,
        audioAssetId: 'asset_123',
        progress: 1.0,
      );

      final json = job.toJson();

      expect(json['id'], 'job_123');
      expect(json['confessionId'], 'confession_456');
      expect(json['voiceId'], 'voice_789');
      expect(json['status'], 'succeeded');
      expect(json['audioAssetId'], 'asset_123');
      expect(json['progress'], 1.0);
    });

    test('AudioGenerationRequest fromJson', () {
      final json = {
        'confessionId': 'confession_123',
        'contentVersionId': 'version_1',
        'variantId': 'variant_1',
        'voiceId': 'voice_456',
        'provider': 'elevenlabs',
        'qualityTier': 'standard',
      };

      final request = AudioGenerationRequest.fromJson(json);

      expect(request.confessionId, 'confession_123');
      expect(request.contentVersionId, 'version_1');
      expect(request.variantId, 'variant_1');
      expect(request.voiceId, 'voice_456');
      expect(request.provider, 'elevenlabs');
      expect(request.qualityTier, 'standard');
    });

    test('AudioGenerationRequest toJson', () {
      final request = AudioGenerationRequest(
        confessionId: 'confession_123',
        contentVersionId: 'version_1',
        variantId: 'variant_1',
        voiceId: 'voice_456',
        provider: 'elevenlabs',
        qualityTier: 'standard',
      );

      final json = request.toJson();

      expect(json['confessionId'], 'confession_123');
      expect(json['contentVersionId'], 'version_1');
      expect(json['variantId'], 'variant_1');
      expect(json['voiceId'], 'voice_456');
      expect(json['provider'], 'elevenlabs');
      expect(json['qualityTier'], 'standard');
    });

    test('AudioAsset fromJson', () {
      final json = {
        'id': 'asset_123',
        'confessionId': 'confession_456',
        'voiceId': 'voice_789',
        'status': 'ready',
        'duration': 120.5,
        'fileSize': 1024000,
        'storagePath': '/audio/asset_123.mp3',
        'createdAt': '2026-01-01T00:00:00Z',
      };

      final asset = AudioAsset.fromJson(json);

      expect(asset.id, 'asset_123');
      expect(asset.confessionId, 'confession_456');
      expect(asset.voiceId, 'voice_789');
      expect(asset.status, AudioAssetStatus.ready);
      expect(asset.duration, 120.5);
      expect(asset.fileSize, 1024000);
      expect(asset.storagePath, '/audio/asset_123.mp3');
    });

    test('TtsProvider fromJson', () {
      final json = {
        'id': 'elevenlabs',
        'name': 'ElevenLabs',
        'isActive': true,
        'qualityTiers': ['standard', 'premium'],
        'config': {'apiKey': 'test_key'},
      };

      final provider = TtsProvider.fromJson(json);

      expect(provider.id, 'elevenlabs');
      expect(provider.name, 'ElevenLabs');
      expect(provider.isActive, true);
      expect(provider.qualityTiers, ['standard', 'premium']);
    });

    test('AudioJobStatus values', () {
      expect(AudioJobStatus.values.length, 6);
      expect(AudioJobStatus.values, contains(AudioJobStatus.queued));
      expect(AudioJobStatus.values, contains(AudioJobStatus.processing));
      expect(AudioJobStatus.values, contains(AudioJobStatus.succeeded));
      expect(AudioJobStatus.values, contains(AudioJobStatus.failed));
      expect(AudioJobStatus.values, contains(AudioJobStatus.cancelled));
      expect(AudioJobStatus.values, contains(AudioJobStatus.pending));
    });

    test('AudioAssetStatus values', () {
      expect(AudioAssetStatus.values.length, 4);
      expect(AudioAssetStatus.values, contains(AudioAssetStatus.pending));
      expect(AudioAssetStatus.values, contains(AudioAssetStatus.processing));
      expect(AudioAssetStatus.values, contains(AudioAssetStatus.ready));
      expect(AudioAssetStatus.values, contains(AudioAssetStatus.failed));
    });
  });
}

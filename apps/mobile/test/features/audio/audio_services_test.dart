import 'dart:collection';

import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:iconfess/src/features/audio/models/audio_generation_job.dart';
import 'package:iconfess/src/features/audio/models/audio_generation_request.dart';
import 'package:iconfess/src/features/audio/services/audio_generation_service.dart';
import 'package:iconfess/src/features/audio/services/audio_url_service.dart';

class FakeApiClient extends ApiClient {
  FakeApiClient()
      : super(
          baseUrl: 'https://api.example.test',
          tokens: InMemoryTokenStore(),
        );

  final Map<String, Map<String, dynamic>> getResponses = {};
  final Map<String, Queue<Map<String, dynamic>>> getSequences = {};
  final Map<String, Map<String, dynamic>> postResponses = {};
  final Map<String, Object?> postBodies = {};
  final List<String> getCalls = [];
  final List<String> postCalls = [];

  @override
  Future<Map<String, dynamic>> get(
    String path, {
    Map<String, String>? query,
  }) async {
    getCalls.add(path);
    final sequence = getSequences[path];
    if (sequence != null && sequence.isNotEmpty) return sequence.removeFirst();
    final response = getResponses[path];
    if (response == null) throw StateError('No fake GET response for $path');
    return response;
  }

  @override
  Future<Map<String, dynamic>> post(String path, [Object? body]) async {
    postCalls.add(path);
    postBodies[path] = body;
    final response = postResponses[path];
    if (response == null) throw StateError('No fake POST response for $path');
    return response;
  }
}

void main() {
  late FakeApiClient client;
  late AudioGenerationService generationService;

  setUp(() {
    client = FakeApiClient();
    generationService = AudioGenerationService(client: client);
  });

  group('AudioGenerationService', () {
    test('creates a job from the server job_id response', () async {
      const request = AudioGenerationRequest(
        confessionId: 'confession-1',
        contentVersionId: 'version-1',
        voiceId: 'voice-1',
      );
      client.postResponses['/admin/audio/generate/job'] = {
        'job_id': 'job-1',
        'status': 'queued',
        'message': 'Job queued for processing',
      };

      final job = await generationService.createJob(request);

      expect(job.id, 'job-1');
      expect(job.confessionId, 'confession-1');
      expect(job.contentVersionId, 'version-1');
      expect(job.status, AudioJobStatus.queued);
      expect(client.postBodies['/admin/audio/generate/job'], request.toApiJson());
    });

    test('parses the server list response wrapped in data', () async {
      client.getResponses['/admin/audio/generate/jobs'] = {
        'data': [
          {'id': 'job-1', 'confession_id': 'confession-1', 'status': 'queued'},
          {'id': 'job-2', 'confession_id': 'confession-2', 'status': 'processing'},
        ],
      };

      final jobs = await generationService.listJobs(limit: 10);

      expect(jobs.map((job) => job.id), ['job-1', 'job-2']);
      expect(client.getCalls.single, '/admin/audio/generate/jobs?limit=10');
    });

    test('loads provider metadata and batch response', () async {
      client.getResponses['/admin/audio/providers'] = {
        'default': 'google',
        'providers': [
          {
            'name': 'google',
            'display_name': 'Google Cloud Text-to-Speech',
            'status': 'available',
            'capabilities': ['neural'],
          },
        ],
      };
      client.postResponses['/admin/audio/generate/batch'] = {
        'message': 'Created 1 generation jobs',
        'job_ids': ['job-1'],
        'total_requested': 1,
        'success_count': 1,
        'fail_count': 0,
      };

      final providers = await generationService.getProviders();
      final batch = await generationService.batchGenerate(
        const BatchAudioGenerationRequest(
          confessionIds: ['confession-1'],
          voiceId: 'voice-1',
        ),
      );

      expect(providers.defaultProvider, 'google');
      expect(providers.providers.single.name, 'google');
      expect(batch.jobIds, ['job-1']);
      expect(batch.successCount, 1);
    });

    test('polls until a job reaches a terminal state', () async {
      const path = '/admin/audio/generate/job/job-1';
      client.getSequences[path] = Queue<Map<String, dynamic>>.of([
        <String, dynamic>{'id': 'job-1', 'status': 'processing'},
        <String, dynamic>{
          'id': 'job-1',
          'status': 'succeeded',
          'audio_asset_id': 'asset-1',
        },
      ]);

      final job = await generationService.pollJobUntilComplete(
        jobId: 'job-1',
        interval: 0,
        timeout: 5,
      );

      expect(job.status, AudioJobStatus.succeeded);
      expect(job.audioAssetId, 'asset-1');
      expect(client.getCalls, [path, path]);
    });
  });

  group('AudioUrlService', () {
    test('resolves only a playable URL from the authorized session queue', () async {
      client.getResponses['/sessions/session-1/queue'] = {
        'session_id': 'session-1',
        'items': [
          {
            'id': 'item-1',
            'audio_asset_id': 'asset-1',
            'audio_url': 'https://cdn.example.test/audio/a.m4a?expires=4102444800&sig=ok',
            'locked': false,
          },
        ],
      };
      final service = AudioUrlService(client: client);

      final url = await service.getStreamUrl('session-1', itemId: 'asset-1');

      expect(url, contains('sig=ok'));
      expect(client.getCalls.single, '/sessions/session-1/queue');
      expect(service.cacheSize, 1);
    });

    test('does not return locked or missing audio URLs', () async {
      client.getResponses['/sessions/session-2/queue'] = {
        'items': [
          {
            'id': 'item-2',
            'audio_asset_id': 'asset-2',
            'audio_url': '',
            'locked': true,
            'lock_reason': 'Premium subscription required',
          },
        ],
      };
      final service = AudioUrlService(client: client);

      await expectLater(
        service.getStreamUrl('session-2', itemId: 'asset-2'),
        throwsA(isA<StateError>().having(
          (error) => error.message,
          'message',
          contains('Premium subscription required'),
        )),
      );
    });
  });
}

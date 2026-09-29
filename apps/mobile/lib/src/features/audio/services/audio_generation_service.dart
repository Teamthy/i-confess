/// Audio generation API client for asynchronous rendering jobs.
import 'dart:async';

import 'package:iconfess_api/iconfess_api.dart';

import '../models/audio_generation_job.dart';
import '../models/audio_generation_request.dart';
import '../models/tts_provider.dart';

class AudioGenerationException implements Exception {
  const AudioGenerationException(
    this.message, {
    this.statusCode,
    this.details,
  });

  final String message;
  final int? statusCode;
  final Object? details;

  @override
  String toString() =>
      'AudioGenerationException: $message${statusCode == null ? '' : ' (HTTP $statusCode)'}';
}

class AudioGenerationService {
  AudioGenerationService({required ApiClient client}) : _client = client;

  final ApiClient _client;

  Future<AudioGenerationJob> createJob(AudioGenerationRequest request) async {
    if (request.contentVersionId == null || request.contentVersionId!.isEmpty) {
      throw ArgumentError.value(
        request.contentVersionId,
        'contentVersionId',
        'A content-version snapshot is required by the generation API.',
      );
    }
    try {
      final response = await _client.postAdminAudioGenerateJob(
        request.toApiJson(),
      );
      return AudioGenerationJob.fromJson({
        ...request.toApiJson(),
        ...response,
        'id': response['id'] ?? response['job_id'] ?? response['jobId'],
      });
    } catch (error) {
      throw _wrap('Failed to create audio generation job', error);
    }
  }

  Future<AudioGenerationJob> getJob(String jobId) async {
    try {
      return AudioGenerationJob.fromJson(
        await _client.getAdminAudioGenerateJobById(jobId),
      );
    } catch (error) {
      throw _wrap('Failed to get audio generation job', error);
    }
  }

  Future<List<AudioGenerationJob>> listJobs({String? status, int? limit}) async {
    try {
      final response = await _client.getAdminAudioGenerateJobs(
        status: status,
        limit: limit,
      );
      final rawJobs = response['jobs'] ?? response['data'];
      if (rawJobs is! List) return const [];
      return rawJobs
          .whereType<Map>()
          .map((job) => AudioGenerationJob.fromJson(
                Map<String, dynamic>.from(job),
              ))
          .toList(growable: false);
    } catch (error) {
      throw _wrap('Failed to list audio generation jobs', error);
    }
  }

  Future<AudioGenerationJob> retryJob(String jobId) async {
    try {
      return AudioGenerationJob.fromJson(
        await _client.postAdminAudioGenerateJobByIdRetry(jobId),
      );
    } catch (error) {
      throw _wrap('Failed to retry audio generation job', error);
    }
  }

  Future<AudioGenerationJob> cancelJob(String jobId) async {
    try {
      return AudioGenerationJob.fromJson(
        await _client.postAdminAudioGenerateJobByIdCancel(jobId),
      );
    } catch (error) {
      throw _wrap('Failed to cancel audio generation job', error);
    }
  }

  Future<AudioGenerationStats> getStats() async {
    try {
      return AudioGenerationStats.fromJson(
        await _client.getAdminAudioGenerateStats(),
      );
    } catch (error) {
      throw _wrap('Failed to get audio generation statistics', error);
    }
  }

  Future<BatchAudioGenerationResponse> batchGenerate(
    BatchAudioGenerationRequest request,
  ) async {
    try {
      return BatchAudioGenerationResponse.fromJson(
        await _client.postAdminAudioGenerateBatch(request.toApiJson()),
      );
    } catch (error) {
      throw _wrap('Failed to trigger batch audio generation', error);
    }
  }

  Future<TtsProvidersResponse> getProviders() async {
    try {
      return TtsProvidersResponse.fromJson(
        await _client.getAdminAudioProviders(),
      );
    } catch (error) {
      throw _wrap('Failed to load text-to-speech providers', error);
    }
  }

  Future<AudioGenerationJob> pollJobUntilComplete({
    required String jobId,
    int interval = 2,
    int timeout = 120,
  }) async {
    if (interval < 0 || timeout <= 0) {
      throw ArgumentError('interval must be non-negative and timeout positive');
    }
    final deadline = DateTime.now().add(Duration(seconds: timeout));
    while (true) {
      final job = await getJob(jobId);
      if (job.isTerminal) return job;
      final remaining = deadline.difference(DateTime.now());
      if (remaining <= Duration.zero) {
        throw AudioGenerationException(
          'Job polling timed out after $timeout seconds',
        );
      }
      await Future<void>.delayed(
        Duration(seconds: interval).compareTo(remaining) < 0
            ? Duration(seconds: interval)
            : remaining,
      );
    }
  }

  Stream<AudioGenerationJob> jobStatusStream({
    required String jobId,
    int interval = 2,
  }) async* {
    if (interval < 0) {
      throw ArgumentError.value(interval, 'interval', 'must not be negative');
    }
    while (true) {
      final job = await getJob(jobId);
      yield job;
      if (job.isTerminal) return;
      await Future<void>.delayed(Duration(seconds: interval));
    }
  }

  AudioGenerationException _wrap(String message, Object error) =>
      AudioGenerationException(
        '$message: $error',
        statusCode: error is ApiError ? error.status : null,
        details: error,
      );
}

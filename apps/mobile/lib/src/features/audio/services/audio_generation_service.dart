/// Audio generation service for managing async audio generation jobs.
///
/// This service handles communication with the backend audio generation API.
import 'dart:async';
import 'package:iconfess_api/iconfess_api.dart';
import '../models/audio_generation_job.dart';
import '../models/audio_generation_request.dart';
import '../models/tts_provider.dart';

/// Exception for audio generation errors.
class AudioGenerationException implements Exception {
  final String message;
  final int? statusCode;
  final dynamic details;

  const AudioGenerationException(this.message, {this.statusCode, this.details});

  @override
  String toString() => 'AudioGenerationException: $message (code: $statusCode)';
}

/// Audio generation service.
class AudioGenerationService {
  final ApiClient _client;

  /// Creates an audio generation service.
  AudioGenerationService({ApiClient? client})
      : _client = client ?? ApiClient();

  /// Queues a new audio generation job.
  ///
  /// Returns the created job with its ID and status.
  Future<AudioGenerationJob> createJob(AudioGenerationRequest request) async {
    try {
      final response = await _client.postAdminAudioGenerateJob(
        request.toApiJson(),
      );
      
      return AudioGenerationJob.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to create audio generation job: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Gets a specific audio generation job by ID.
  Future<AudioGenerationJob> getJob(String jobId) async {
    try {
      final response = await _client.getAdminAudioGenerateJobById(jobId);
      return AudioGenerationJob.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to get audio generation job: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Lists all audio generation jobs.
  ///
  /// [status] - Optional filter by job status.
  /// [limit] - Optional limit on the number of jobs returned.
  Future<List<AudioGenerationJob>> listJobs({
    String? status,
    int? limit,
  }) async {
    try {
      final response = await _client.getAdminAudioGenerateJobs(
        status: status,
        limit: limit,
      );
      
      final jobs = (response['jobs'] as List<dynamic>? ?? [])
          .map((json) => AudioGenerationJob.fromJson(json))
          .toList();
      
      return jobs;
    } catch (e) {
      throw AudioGenerationException(
        'Failed to list audio generation jobs: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Retries a failed audio generation job.
  Future<AudioGenerationJob> retryJob(String jobId) async {
    try {
      final response = await _client.postAdminAudioGenerateJobByIdRetry(jobId);
      return AudioGenerationJob.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to retry audio generation job: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Cancels a pending audio generation job.
  Future<AudioGenerationJob> cancelJob(String jobId) async {
    try {
      final response = await _client.postAdminAudioGenerateJobByIdCancel(jobId);
      return AudioGenerationJob.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to cancel audio generation job: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Gets audio generation statistics.
  Future<AudioGenerationStats> getStats() async {
    try {
      final response = await _client.getAdminAudioGenerateStats();
      return AudioGenerationStats.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to get audio generation stats: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Triggers batch audio generation for multiple confessions.
  Future<BatchAudioGenerationResponse> batchGenerate(
    BatchAudioGenerationRequest request,
  ) async {
    try {
      final response = await _client.postAdminAudioGenerateBatch(
        request.toApiJson(),
      );
      return BatchAudioGenerationResponse.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to trigger batch audio generation: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Gets the list of available TTS providers.
  Future<TtsProvidersResponse> getProviders() async {
    try {
      final response = await _client.getAdminAudioProviders();
      return TtsProvidersResponse.fromJson(response);
    } catch (e) {
      throw AudioGenerationException(
        'Failed to get TTS providers: $e',
        statusCode: e is ApiException ? e.statusCode : null,
        details: e,
      );
    }
  }

  /// Polls a job until it reaches a terminal state.
  ///
  /// [jobId] - The ID of the job to poll.
  /// [interval] - The polling interval in seconds.
  /// [timeout] - The maximum time to wait in seconds.
  ///
  /// Returns the job when it reaches a terminal state.
  Future<AudioGenerationJob> pollJobUntilComplete({
    required String jobId,
    int interval = 2,
    int timeout = 120,
  }) async {
    final startTime = DateTime.now();
    
    while (true) {
      final job = await getJob(jobId);
      
      if (job.isTerminal) {
        return job;
      }
      
      // Check if we've exceeded the timeout
      final elapsed = DateTime.now().difference(startTime).inSeconds;
      if (elapsed >= timeout) {
        throw AudioGenerationException(
          'Job polling timed out after $timeout seconds',
        );
      }
      
      // Wait before polling again
      await Future.delayed(Duration(seconds: interval));
    }
  }

  /// Gets the status of a job as a stream.
  ///
  /// Polls the job status at the specified interval and emits updates.
  Stream<AudioGenerationJob> jobStatusStream({
    required String jobId,
    int interval = 2,
  }) async* {
    while (true) {
      final job = await getJob(jobId);
      yield job;
      
      if (job.isTerminal) {
        break;
      }
      
      await Future.delayed(Duration(seconds: interval));
    }
  }
}

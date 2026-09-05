import 'dart:convert';
import 'package:http/http.dart' as http;

/// TrialService fetches the 7-day journey from GET /v1/subscriptions/trial.
/// Day1 Morning … Day7 Weekly Summary. Mobile never invents the journey.
class TrialService {
  TrialService({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();
  final String baseUrl;
  final http.Client _client;

  Future<TrialState> fetchTrial(String authToken) async {
    final resp = await _client.get(
      Uri.parse('$baseUrl/v1/subscriptions/trial'),
      headers: {'Authorization': 'Bearer $authToken', 'Accept': 'application/json'},
    );
    if (resp.statusCode != 200) throw Exception('trial HTTP ${resp.statusCode}: ${resp.body}');
    return TrialState.fromJson(jsonDecode(resp.body) as Map<String, dynamic>);
  }
}

class TrialState {
  TrialState({required this.journey, required this.currentDay, this.today});
  final List<TrialDay> journey;
  final int currentDay; // 0 = not in trial or complete
  final TrialDay? today;
  bool get inTrial => currentDay >= 1 && currentDay <= 7;

  factory TrialState.fromJson(Map<String, dynamic> j) => TrialState(
        journey: (j['journey'] as List).map((e) => TrialDay.fromJson(e as Map<String, dynamic>)).toList(),
        currentDay: (j['current_day'] as num).toInt(),
        today: j['today'] != null ? TrialDay.fromJson(j['today'] as Map<String, dynamic>) : null,
      );
}

class TrialDay {
  TrialDay({required this.day, required this.title, required this.description, required this.categories, required this.duration});
  final int day;
  final String title;
  final String description;
  final List<String> categories;
  final int duration; // seconds

  factory TrialDay.fromJson(Map<String, dynamic> j) => TrialDay(
        day: (j['day'] as num).toInt(),
        title: j['title'] as String,
        description: j['description'] as String,
        categories: (j['categories'] as List).cast<String>(),
        duration: (j['duration'] as num).toInt(),
      );
}

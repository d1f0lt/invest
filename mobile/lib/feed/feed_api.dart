import '../api/api_client.dart';
import '../api/securities_api.dart';

enum FeedKind {
  all('all', 'Все', 'Публикация'),
  news('news', 'Новости', 'Новость'),
  forecast('forecast', 'Прогнозы', 'Прогноз');

  const FeedKind(this.value, this.label, this.postLabel);

  final String value;
  final String label;
  final String postLabel;
}

enum NewsStatus {
  any('', 'Все'),
  verified('verified', 'Проверенные'),
  unverified('unverified', 'Непроверенные');

  const NewsStatus(this.value, this.label);

  final String value;
  final String label;
}

enum Recommendation {
  buy('buy', 'Покупать'),
  sell('sell', 'Продавать'),
  hold('hold', 'Держать');

  const Recommendation(this.value, this.label);

  final String value;
  final String label;

  bool allows(double change) => switch (this) {
    buy => change > 0,
    sell => change < 0,
    hold => true,
  };
}

class FeedPost {
  FeedPost.fromJson(Map<String, dynamic> json)
    : id = json['id'] as int,
      kind = FeedKind.values.firstWhere((kind) => kind.value == json['kind']),
      title = json['title'] as String,
      body = json['body'] as String,
      author = json['author'] as String,
      authorIsAdmin = json['author_is_admin'] as bool?,
      tickers = (json['tickers'] as List).cast<String>(),
      board = json['board'] as String? ?? '',
      createdAt = DateTime.parse(json['created_at'] as String),
      verified = json['verified'] as bool,
      rating = json['rating'] as int?,
      ratingSamples = json['rating_samples'] as int,
      change = (json['change_percent'] as num?)?.toDouble(),
      targetDate = DateTime.tryParse(json['target_date'] as String? ?? ''),
      recommendation = Recommendation.values
          .where((value) => value.value == json['recommendation'])
          .firstOrNull;

  final int id;
  final FeedKind kind;
  final String title;
  final String body;
  final String author;
  final bool? authorIsAdmin;
  final List<String> tickers;
  final String board;
  final DateTime createdAt;
  final bool verified;
  final int? rating;
  final int ratingSamples;
  final double? change;
  final DateTime? targetDate;
  final Recommendation? recommendation;
}

class FeedComment {
  FeedComment.fromJson(Map<String, dynamic> json)
    : id = json['id'] as int,
      author = json['author'] as String,
      authorIsAdmin = json['author_is_admin'] as bool?,
      canDelete = json['can_delete'] == true,
      body = json['body'] as String,
      createdAt = DateTime.parse(json['created_at'] as String);

  final int id;
  final String author;
  final bool? authorIsAdmin;
  final bool canDelete;
  final String body;
  final DateTime createdAt;
}

class FeedReactions {
  FeedReactions.fromJson(Map<String, dynamic> json)
    : likes = json['likes'] as int,
      dislikes = json['dislikes'] as int,
      mine = json['mine'] as int;

  final int likes;
  final int dislikes;
  final int mine;
}

class FeedApi {
  final _client = ApiClient();

  Future<({List<FeedPost> posts, int? next, String? nextAt})> list({
    required FeedKind kind,
    required NewsStatus status,
    required List<String> tickers,
    int? minRating,
    int? minNewsRating,
    int? minForecastRating,
    int? before,
    String? beforeAt,
  }) async {
    final path = Uri(
      path: '/api/v1/feed',
      queryParameters: {
        'kind': kind.value,
        if (status != NewsStatus.any) 'status': status.value,
        if (tickers.isNotEmpty) 'ticker': tickers,
        if (minRating != null) 'min_rating': minRating.toString(),
        if (minNewsRating != null) 'min_news_rating': minNewsRating.toString(),
        if (minForecastRating != null)
          'min_forecast_rating': minForecastRating.toString(),
        if (before != null) 'before': before.toString(),
        'before_at': ?beforeAt,
      },
    );
    final json =
        await _client.get(path.toString(), auth: true) as Map<String, dynamic>;
    return (
      posts: (json['posts'] as List)
          .map((item) => FeedPost.fromJson(item as Map<String, dynamic>))
          .toList(),
      next: json['next'] as int?,
      nextAt: json['next_at'] as String?,
    );
  }

  Future<
    ({
      List<FeedComment> comments,
      int? next,
      FeedReactions reactions,
      bool canModerate,
      bool canDeletePost,
      bool? verified,
    })
  >
  discussion(int postId, {int? before}) async {
    final uri = Uri(
      path: '/api/v1/feed/$postId/discussion',
      queryParameters: {if (before != null) 'before': before.toString()},
    );
    final json =
        await _client.get(uri.toString(), auth: true) as Map<String, dynamic>;
    return (
      comments: (json['comments'] as List)
          .map((value) => FeedComment.fromJson(value as Map<String, dynamic>))
          .toList(),
      canModerate: json['can_moderate'] == true,
      canDeletePost:
          json['can_delete_post'] == true || json['can_moderate'] == true,
      verified: json['verified'] as bool?,
      next: json['next'] as int?,
      reactions: FeedReactions.fromJson(
        json['reactions'] as Map<String, dynamic>,
      ),
    );
  }

  Future<void> moderate(int postId, String action, {int? commentId}) async {
    await _client.post('/api/v1/feed/$postId/moderation', {
      'action': action,
      'comment_id': ?commentId,
    }, auth: true);
  }

  Future<FeedComment> comment(int postId, String body) async {
    final json = await _client.post('/api/v1/feed/$postId/comments', {
      'body': body,
    }, auth: true) as Map<String, dynamic>;
    return FeedComment.fromJson(json);
  }

  Future<FeedReactions> react(int postId, int value) async {
    final json = await _client.put('/api/v1/feed/$postId/reaction', {
      'value': value,
    }, auth: true) as Map<String, dynamic>;
    return FeedReactions.fromJson(json);
  }

  Future<FeedPost?> savePost({
    int? postId,
    required FeedKind kind,
    required String title,
    required String body,
    required List<Security> securities,
    double? change,
    Recommendation? recommendation,
    DateTime? targetDate,
  }) async {
    final payload = <String, dynamic>{
      'kind': kind.value,
      'title': title,
      'body': body,
      'tickers': [for (final item in securities) item.secid],
      if (kind == FeedKind.forecast) ...{
        'board': securities.single.board,
        'change_percent': change,
        'recommendation': recommendation?.value,
        'target_date': targetDate?.toIso8601String().split('T').first,
      },
    };
    if (postId == null) {
      await _client.post('/api/v1/feed', payload, auth: true);
      return null;
    }
    final json = await _client.patch(
      '/api/v1/feed/$postId',
      payload,
      auth: true,
    ) as Map<String, dynamic>;
    return FeedPost.fromJson(json);
  }
}

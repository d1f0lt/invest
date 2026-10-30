import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import '../profile/user_role_label.dart';
import 'feed_api.dart';
import 'feed_compose_screen.dart';
import 'feed_security_picker.dart';
import 'feed_post_screen.dart';
import 'feed_post_style.dart';

class FeedTab extends StatefulWidget {
  const FeedTab({super.key, required this.active});

  final bool active;

  @override
  State<FeedTab> createState() => _FeedTabState();
}

class _FeedTabState extends State<FeedTab> {
  final _api = FeedApi();
  FeedKind _kind = FeedKind.all;
  NewsStatus _status = NewsStatus.any;
  List<Security> _securities = [];
  int? _minRating;
  int? _minNewsRating;
  int? _minForecastRating;
  List<FeedPost> _posts = [];
  int? _next;
  String? _nextAt;
  int _request = 0;
  bool _loading = true;
  bool _loadingMore = false;
  bool _showFilters = false;
  String? _error;

  bool get _showRating =>
      _kind == FeedKind.forecast || _status == NewsStatus.unverified;

  @override
  void didUpdateWidget(covariant FeedTab oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.active && !oldWidget.active) _load();
  }

  Future<void> _load({bool more = false}) async {
    if (more && (_loadingMore || _next == null)) return;
    final request = ++_request;
    setState(() {
      _loading = !more;
      _loadingMore = more;
      _error = null;
      if (!more) _next = null;
    });
    try {
      final page = await _api.list(
        kind: _kind,
        status: _status,
        tickers: [for (final security in _securities) security.secid],
        minRating: _showRating ? _minRating : null,
        minNewsRating: _kind == FeedKind.all ? _minNewsRating : null,
        minForecastRating: _kind == FeedKind.all ? _minForecastRating : null,
        before: more ? _next : null,
        beforeAt: more ? _nextAt : null,
      );
      if (!mounted || request != _request) return;
      setState(() {
        _posts = more ? [..._posts, ...page.posts] : page.posts;
        _next = page.next;
        _nextAt = page.nextAt;
      });
    } catch (e) {
      if (mounted && request == _request) {
        setState(
          () => _error = e is ApiException
              ? e.message
              : 'Не удалось загрузить ленту',
        );
      }
    } finally {
      if (mounted && request == _request) {
        setState(() {
          _loading = false;
          _loadingMore = false;
        });
      }
    }
  }

  void _changeFilters(VoidCallback update) {
    setState(() {
      update();
      _posts = [];
    });
    _load();
  }

  Future<void> _publish() async {
    final published = await Navigator.of(context).push<FeedComposeResult>(
      MaterialPageRoute(builder: (_) => FeedComposeScreen(kind: _kind)),
    );
    if (!mounted || published == null) return;
    _changeFilters(() {
      if (_kind != FeedKind.all) _kind = published.kind;
      _minNewsRating = null;
      _minForecastRating = null;
      _status = NewsStatus.any;
      _securities = [];
      _minRating = null;
    });
    ScaffoldMessenger.of(context)
        .showSnackBar(const SnackBar(content: Text('Публикация добавлена')));
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Лента'),
        actions: [
          IconButton(
            tooltip: _showFilters ? 'Скрыть фильтры' : 'Показать фильтры',
            isSelected: _showFilters,
            icon: const Icon(Icons.tune_outlined),
            selectedIcon: const Icon(Icons.tune_rounded),
            onPressed: () => setState(() => _showFilters = !_showFilters),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton(
        tooltip: 'Новая публикация',
        onPressed: _publish,
        child: const Icon(Icons.add),
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 88),
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            Wrap(
              spacing: 8,
              children: [
                for (final kind in FeedKind.values)
                  ChoiceChip(
                    label: Text(kind.label),
                    selected: _kind == kind,
                    onSelected: (selected) {
                      if (!selected || _kind == kind) return;
                      _changeFilters(() {
                        _kind = kind;
                        _status = NewsStatus.any;
                        _minRating = null;
                        _securities = [];
                      });
                    },
                  ),
              ],
            ),
            if (_showFilters) _filters(context),
            if (_loading || _loadingMore)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 12),
                child: LinearProgressIndicator(),
              ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16),
                child: Column(
                  children: [
                    Text(
                      _error!,
                      style: TextStyle(color: theme.colorScheme.error),
                    ),
                    TextButton(
                      onPressed: () => _load(more: _next != null),
                      child: const Text('Повторить'),
                    ),
                  ],
                ),
              ),
            if (!_loading && _error == null && _posts.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 40),
                child: Center(
                  child: Text(switch (_kind) {
                    FeedKind.all => 'Публикаций пока нет',
                    FeedKind.news => 'Новостей пока нет',
                    FeedKind.forecast => 'Активных прогнозов пока нет',
                  }, style: theme.textTheme.bodyLarge),
                ),
              ),
            for (final post in _posts)
              _FeedCard(post: post, onModerated: _load),
            if (_next != null && !_loadingMore && _error == null)
              TextButton(
                onPressed: () => _load(more: true),
                child: const Text('Загрузить ещё'),
              ),
          ],
        ),
      ),
    );
  }

  Widget _filters(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (_kind == FeedKind.news)
            Wrap(
              spacing: 8,
              children: [
                for (final status in NewsStatus.values)
                  ChoiceChip(
                    label: Text(status.label),
                    selected: _status == status,
                    onSelected: (selected) {
                      if (!selected || _status == status) return;
                      _changeFilters(() {
                        _status = status;
                        _minRating = null;
                      });
                    },
                  ),
              ],
            ),
          FeedSecurityPicker(
            selected: _securities,
            maximum: _kind == FeedKind.forecast ? 1 : 20,
            onChanged: (items) => _changeFilters(() => _securities = items),
          ),
          if (_kind == FeedKind.all) ...[
            _ratingFilter(
              'Рейтинг за новости',
              _minNewsRating,
              (value) => _minNewsRating = value,
            ),
            _ratingFilter(
              'Рейтинг за прогнозы',
              _minForecastRating,
              (value) => _minForecastRating = value,
            ),
          ] else if (_showRating)
            _ratingFilter(
              _kind == FeedKind.news
                  ? 'Рейтинг за новости'
                  : 'Рейтинг за прогнозы',
              _minRating,
              (value) => _minRating = value,
            ),
        ],
      ),
    );
  }

  Widget _ratingFilter(
    String label,
    int? rating,
    ValueChanged<int?> onChanged,
  ) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label),
          DropdownButton<int>(
            isExpanded: true,
            value: rating ?? -1,
            items: [
              const DropdownMenuItem(value: -1, child: Text('Любой')),
              for (final minimum in [50, 75, 90, 95])
                DropdownMenuItem(value: minimum, child: Text('$minimum%+')),
            ],
            onChanged: (value) =>
                _changeFilters(() => onChanged(value == -1 ? null : value)),
          ),
        ],
      ),
    );
  }
}

class _FeedCard extends StatelessWidget {
  const _FeedCard({required this.post, required this.onModerated});

  final FeedPost post;
  final VoidCallback onModerated;

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(top: 12),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: () => Navigator.of(context).push<void>(
          MaterialPageRoute(
            builder: (_) =>
                FeedPostScreen(post: post, onModerated: onModerated),
          ),
        ),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: _heading(context),
        ),
      ),
    );
  }

  Widget _heading(BuildContext context) {
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    final dark = theme.brightness == Brightness.dark;
    final isNews = post.kind == FeedKind.news;
    final newsColor = dark ? const Color(0xFF90CAF9) : const Color(0xFF175BA3);
    final change = post.change;
    final targetDate = post.targetDate;
    final recommendation = post.recommendation;
    final rating = post.rating;
    final metaStyle = theme.textTheme.bodySmall?.copyWith(
      color: colors.onSurfaceVariant,
    );
    final labelStyle = theme.textTheme.labelSmall?.copyWith(
      fontWeight: FontWeight.w600,
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              flex: 3,
              child: Text(post.title, style: theme.textTheme.titleMedium),
            ),
            const SizedBox(width: 12),
            Flexible(
              child: Align(
                alignment: AlignmentDirectional.topEnd,
                child: Text(
                  post.kind.postLabel,
                  textAlign: TextAlign.end,
                  style: labelStyle?.copyWith(
                    color: isNews ? newsColor : colors.primary,
                  ),
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 6),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: Wrap(
                spacing: 8,
                runSpacing: 2,
                children: [
                  Text(post.author, style: metaStyle),
                  UserRoleLabel(isAdmin: post.authorIsAdmin),
                ],
              ),
            ),
            const SizedBox(width: 12),
            Flexible(
              child: Align(
                alignment: AlignmentDirectional.topEnd,
                child: Tooltip(
                  message: rating == null
                      ? 'Рейтинг пока не рассчитан'
                      : 'Оценок: ${post.ratingSamples}',
                  child: Text(
                    rating == null ? 'Нет рейтинга' : 'Рейтинг $rating%',
                    textAlign: TextAlign.end,
                    style: metaStyle,
                  ),
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: [
            for (final ticker in post.tickers) FeedTickerLabel(ticker: ticker),
          ],
        ),
        if (isNews)
          Align(
            alignment: AlignmentDirectional.centerEnd,
            child: Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                post.verified ? 'Проверена' : 'Непроверена',
                style: labelStyle?.copyWith(
                  color: post.verified
                      ? feedPositiveColor(theme)
                      : colors.onSurfaceVariant,
                ),
              ),
            ),
          ),
        if (change != null && targetDate != null && recommendation != null)
          Padding(
            padding: const EdgeInsets.only(top: 10),
            child: Wrap(
              spacing: 10,
              runSpacing: 4,
              children: [
                Text.rich(
                  TextSpan(
                    children: [
                      TextSpan(
                        text: feedPercentText(change),
                        style: TextStyle(
                          color: feedChangeColor(theme, change),
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      TextSpan(
                        text:
                            ' до ${MaterialLocalizations.of(context).formatCompactDate(targetDate)}',
                      ),
                    ],
                  ),
                ),
                Text(
                  recommendation.label,
                  style: theme.textTheme.bodyMedium?.copyWith(
                    fontWeight: FontWeight.w700,
                    color: feedRecommendationColor(theme, recommendation),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

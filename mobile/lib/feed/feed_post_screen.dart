import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../profile/user_role_label.dart';
import 'feed_api.dart';
import 'feed_compose_screen.dart';
import 'feed_post_style.dart';

class FeedPostScreen extends StatefulWidget {
  const FeedPostScreen({
    super.key,
    required this.post,
    required this.onModerated,
  });

  final FeedPost post;
  final VoidCallback onModerated;

  @override
  State<FeedPostScreen> createState() => _FeedPostScreenState();
}

class _FeedPostScreenState extends State<FeedPostScreen> {
  final _api = FeedApi();
  final _comment = TextEditingController();
  final _commentFocus = FocusNode();
  final _comments = <FeedComment>[];
  FeedReactions? _reactions;
  int? _next;
  String? _loadError;
  bool _loading = false;
  bool _reacting = false;
  bool _sending = false;
  bool _canModerate = false;
  bool _canDeletePost = false;
  bool _moderating = false;
  late FeedPost _post = widget.post;
  late bool _verified = _post.verified;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _comment.dispose();
    _commentFocus.dispose();
    super.dispose();
  }

  Future<void> _load({int? before}) async {
    setState(() {
      _loading = true;
      _loadError = null;
    });
    try {
      final page = await _api.discussion(widget.post.id, before: before);
      if (!mounted) return;
      setState(() {
        if (before == null) {
          _comments.clear();
          _reactions = page.reactions;
          _canModerate = page.canModerate;
          _canDeletePost = page.canDeletePost;
          _verified = page.verified ?? _verified;
        }
        _comments.addAll(page.comments);
        _next = page.next;
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => _loadError = _errorText(e, 'Не удалось загрузить обсуждение'),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _react(int value) async {
    final selected = _reactions?.mine == value;
    setState(() => _reacting = true);
    try {
      final reactions = await _api.react(widget.post.id, selected ? 0 : value);
      if (mounted) setState(() => _reactions = reactions);
    } catch (e) {
      if (mounted) _showError(e, 'Не удалось сохранить реакцию');
    } finally {
      if (mounted) setState(() => _reacting = false);
    }
  }

  Future<void> _send() async {
    final text = _comment.text.trim();
    if (text.isEmpty) return;
    setState(() => _sending = true);
    try {
      final comment = await _api.comment(widget.post.id, text);
      if (!mounted) return;
      _comment.clear();
      FocusScope.of(context).unfocus();
      setState(() => _comments.insert(0, comment));
    } catch (e) {
      if (mounted) _showError(e, 'Не удалось отправить комментарий');
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  String _errorText(Object error, String fallback) => error is ApiException
      ? error.statusCode == 422
            ? error.serverMessage ?? error.message
            : error.message
      : fallback;

  void _showError(Object error, String fallback) {
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(_errorText(error, fallback))));
  }

  @override
  Widget build(BuildContext context) {
    final post = _post;
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    final localizations = MaterialLocalizations.of(context);
    final publicationDate = localizations.formatFullDate(
      post.createdAt.toLocal(),
    );
    final typeScale = MediaQuery.textScalerOf(context);
    return Scaffold(
      appBar: AppBar(
        toolbarHeight: typeScale.scale(20) * 3.8,
        title: Tooltip(
          message: post.title,
          child: Text(post.title, maxLines: 3, overflow: TextOverflow.ellipsis),
        ),
      ),
      body: SafeArea(
        top: false,
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 32),
                children: [
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      for (final ticker in post.tickers)
                        FeedTickerLabel(ticker: ticker),
                    ],
                  ),
                  if (post.recommendation case final recommendation?) ...[
                    const SizedBox(height: 16),
                    _PostFact(
                      label: 'Рекомендация',
                      value: recommendation.label,
                      color: feedRecommendationColor(theme, recommendation),
                    ),
                  ],
                  if (post.change case final change?) ...[
                    const SizedBox(height: 6),
                    _PostFact(
                      label: change > 0
                          ? 'Рост'
                          : change < 0
                          ? 'Снижение'
                          : 'Изменение',
                      value: feedPercentText(change),
                      color: feedChangeColor(theme, change),
                    ),
                  ],
                  if (post.targetDate case final date?) ...[
                    const SizedBox(height: 6),
                    Text(
                      'Срок прогноза до ${localizations.formatCompactDate(date)}',
                    ),
                  ],
                  const SizedBox(height: 20),
                  SelectableText(
                    post.body,
                    style: theme.textTheme.bodyLarge?.copyWith(fontSize: 18),
                  ),
                  const SizedBox(height: 12),
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.baseline,
                    textBaseline: TextBaseline.alphabetic,
                    children: [
                      Expanded(
                        child: Text(
                          publicationDate,
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: colors.onSurfaceVariant,
                          ),
                        ),
                      ),
                      if (post.kind == FeedKind.news) ...[
                        const SizedBox(width: 12),
                        Flexible(
                          child: Align(
                            alignment: AlignmentDirectional.topEnd,
                            child: Text(
                              _verified ? 'Проверена' : 'Непроверена',
                              textAlign: TextAlign.end,
                              style: theme.textTheme.labelSmall?.copyWith(
                                fontWeight: FontWeight.w600,
                                color: _verified
                                    ? feedPositiveColor(theme)
                                    : colors.onSurfaceVariant,
                              ),
                            ),
                          ),
                        ),
                      ],
                    ],
                  ),
                  const SizedBox(height: 12),
                  if (_reactions case final reactions?)
                    _reactionButtons(reactions),
                  const SizedBox(height: 8),
                  _AuthorCard(post: post),
                  const Divider(height: 32),
                  Text('Комментарии', style: theme.textTheme.titleMedium),
                  const SizedBox(height: 12),
                  if (_loading) const LinearProgressIndicator(),
                  if (_loadError case final error?)
                    Column(
                      children: [
                        Text(error, style: TextStyle(color: colors.error)),
                        TextButton(
                          onPressed: () => _load(before: _next),
                          child: const Text('Повторить'),
                        ),
                      ],
                    ),
                  if (!_loading && _loadError == null && _comments.isEmpty)
                    Text(
                      'Комментариев пока нет',
                      style: theme.textTheme.bodyMedium,
                    ),
                  for (final comment in _comments)
                    _CommentCard(
                      comment: comment,
                      onDelete: comment.canDelete && !_moderating
                          ? () => _moderate('delete_comment', comment: comment)
                          : null,
                    ),
                  if (_next != null && !_loading && _loadError == null)
                    TextButton(
                      onPressed: () => _load(before: _next),
                      child: const Text('Ещё комментарии'),
                    ),
                  if (_canModerate || _canDeletePost)
                    _moderationSection(context),
                  const SizedBox(height: 16),
                  Text(
                    'Не является инвестиционной рекомендацией',
                    textAlign: TextAlign.center,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: colors.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            if (_reactions != null) _commentComposer(context),
          ],
        ),
      ),
    );
  }

  Future<void> _moderate(String action, {FeedComment? comment}) async {
    if (_moderating) return;
    if (action != 'verify_news') {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: Text(
            comment == null ? 'Удалить публикацию?' : 'Удалить комментарий?',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Отмена'),
            ),
            TextButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Удалить'),
            ),
          ],
        ),
      );
      if (!mounted || confirmed != true) return;
    }
    setState(() => _moderating = true);
    try {
      await _api.moderate(widget.post.id, action, commentId: comment?.id);
      if (!mounted) return;
      if (comment != null) {
        setState(() => _comments.removeWhere((item) => item.id == comment.id));
      } else {
        widget.onModerated();
        if (action == 'delete_post') {
          Navigator.of(context).pop();
        } else {
          setState(() => _verified = true);
        }
      }
    } catch (error) {
      if (mounted) _showError(error, 'Не удалось выполнить модерацию');
    } finally {
      if (mounted) setState(() => _moderating = false);
    }
  }

  Future<void> _editPost() async {
    final result = await Navigator.of(context).push<FeedComposeResult>(
      MaterialPageRoute(
        builder: (_) => FeedComposeScreen(kind: _post.kind, post: _post),
      ),
    );
    if (!mounted || result?.updated == null) return;
    final updated = result!.updated!;
    setState(() {
      _post = updated;
      _verified = updated.verified;
    });
    widget.onModerated();
  }

  Widget _moderationSection(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Divider(height: 32),
        if (_canModerate) Text('Модерация', style: theme.textTheme.titleMedium),
        Wrap(
          spacing: 8,
          children: [
            if (_canModerate && widget.post.kind == FeedKind.news && !_verified)
              TextButton.icon(
                onPressed: _moderating ? null : () => _moderate('verify_news'),
                icon: const Icon(Icons.verified_outlined),
                label: const Text('Подтвердить новость'),
              ),
            if (_canDeletePost)
              TextButton.icon(
                onPressed: _moderating ? null : _editPost,
                icon: const Icon(Icons.edit_outlined),
                label: const Text('Редактировать публикацию'),
              ),
            if (_canDeletePost)
              TextButton.icon(
                onPressed: _moderating ? null : () => _moderate('delete_post'),
                icon: const Icon(Icons.delete_outline),
                label: const Text('Удалить публикацию'),
                style: TextButton.styleFrom(
                  foregroundColor: theme.colorScheme.error,
                ),
              ),
          ],
        ),
      ],
    );
  }

  Widget _commentComposer(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Material(
      color: colors.surface,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
        child: ListenableBuilder(
          listenable: Listenable.merge([_commentFocus, _comment]),
          builder: (context, _) {
            final focused = _commentFocus.hasFocus;
            return Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Expanded(
                  child: TextField(
                    controller: _comment,
                    focusNode: _commentFocus,
                    readOnly: _sending,
                    minLines: 1,
                    maxLines: focused ? 4 : 1,
                    maxLength: 2000,
                    decoration: InputDecoration(
                      hintText: 'Введите текст комментария',
                      counterText: '',
                      filled: true,
                      fillColor: colors.surfaceContainer,
                      isDense: true,
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(20),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),
                ),
                if (focused || _sending)
                  IconButton(
                    tooltip: 'Отправить комментарий',
                    onPressed: _sending || _comment.text.trim().isEmpty
                        ? null
                        : _send,
                    icon: _sending
                        ? const SizedBox.square(
                            dimension: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.send_rounded, size: 20),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _reactionButtons(FeedReactions reactions) {
    return Wrap(
      spacing: 8,
      runSpacing: 4,
      children: [
        Tooltip(
          message: 'Нравится',
          child: TextButton.icon(
            icon: Icon(
              reactions.mine == 1 ? Icons.thumb_up : Icons.thumb_up_outlined,
              size: 18,
            ),
            label: Text('${reactions.likes}'),
            onPressed: _reacting ? null : () => _react(1),
          ),
        ),
        Tooltip(
          message: 'Не нравится',
          child: TextButton.icon(
            icon: Icon(
              reactions.mine == -1
                  ? Icons.thumb_down
                  : Icons.thumb_down_outlined,
              size: 18,
            ),
            label: Text('${reactions.dislikes}'),
            onPressed: _reacting ? null : () => _react(-1),
          ),
        ),
      ],
    );
  }
}

class _PostFact extends StatelessWidget {
  const _PostFact({
    required this.label,
    required this.value,
    required this.color,
  });

  final String label;
  final String value;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final style = Theme.of(context).textTheme.bodyMedium;
    return Text.rich(
      TextSpan(
        text: '$label: ',
        children: [
          TextSpan(
            text: value,
            style: TextStyle(color: color, fontWeight: FontWeight.w700),
          ),
        ],
      ),
      style: style,
    );
  }
}

class _CommentCard extends StatelessWidget {
  const _CommentCard({required this.comment, this.onDelete});

  final FeedComment comment;
  final VoidCallback? onDelete;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 5),
      elevation: 0,
      color: colors.surfaceContainerLow,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: colors.outlineVariant),
      ),
      child: Stack(
        children: [
          Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    _FeedAvatar(name: comment.author, radius: 16),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        comment.author,
                        style: theme.textTheme.titleSmall,
                      ),
                    ),
                    if (comment.authorIsAdmin != null)
                      Flexible(
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 8),
                          child: Align(
                            alignment: AlignmentDirectional.centerEnd,
                            child: UserRoleLabel(
                              isAdmin: comment.authorIsAdmin,
                            ),
                          ),
                        ),
                      ),
                    if (comment.canDelete) const SizedBox(width: 28),
                  ],
                ),
                const SizedBox(height: 8),
                SelectableText(comment.body, style: theme.textTheme.bodyLarge),
                const SizedBox(height: 6),
                Text(
                  MaterialLocalizations.of(context)
                      .formatShortDate(comment.createdAt.toLocal()),
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: colors.onSurfaceVariant,
                  ),
                ),
              ],
            ),
          ),
          if (comment.canDelete)
            PositionedDirectional(
              top: 0,
              end: 0,
              child: IconButton(
                tooltip: 'Удалить комментарий',
                onPressed: onDelete,
                color: colors.error,
                alignment: AlignmentDirectional.topEnd,
                padding: const EdgeInsets.all(6),
                constraints: const BoxConstraints.tightFor(
                  width: 48,
                  height: 48,
                ),
                icon: const Icon(Icons.close, size: 16),
              ),
            ),
        ],
      ),
    );
  }
}

class _FeedAvatar extends StatelessWidget {
  const _FeedAvatar({required this.name, this.radius = 20});

  final String name;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    final trimmed = name.trim();
    return CircleAvatar(
      radius: radius,
      backgroundColor: colors.primaryContainer,
      foregroundColor: colors.onPrimaryContainer,
      child: trimmed.isEmpty
          ? const Icon(Icons.person_outline, size: 18)
          : Text(trimmed.characters.first.toUpperCase()),
    );
  }
}

class _AuthorCard extends StatelessWidget {
  const _AuthorCard({required this.post});

  final FeedPost post;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    final name = post.author.trim();
    final rating = post.rating;
    return DecoratedBox(
      decoration: BoxDecoration(
        color: colors.surfaceContainerLow,
        border: Border.all(color: colors.outlineVariant),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          children: [
            _FeedAvatar(name: name),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Wrap(
                    spacing: 8,
                    runSpacing: 2,
                    children: [
                      Text(name, style: theme.textTheme.titleSmall),
                      UserRoleLabel(isAdmin: post.authorIsAdmin),
                    ],
                  ),
                  Text(
                    rating == null
                        ? 'Нет рейтинга'
                        : 'Рейтинг $rating% · оценок ${post.ratingSamples}',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: colors.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

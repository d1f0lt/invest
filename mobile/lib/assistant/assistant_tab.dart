import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../config.dart';
import '../feed/feed_api.dart';
import '../portfolio/portfolio_picker.dart';
import '../portfolio/portfolio_store.dart';

class AssistantTab extends StatefulWidget {
  const AssistantTab({super.key});

  @override
  State<AssistantTab> createState() => _AssistantTabState();
}

class _AssistantTabState extends State<AssistantTab> {
  final _api = ApiClient(baseUrl: AppConfig.assistantBaseUrl);
  final _question = TextEditingController();
  final _scroll = ScrollController();
  final _store = PortfolioStore.instance;
  final _turns = <_Turn>[];
  Portfolio? _portfolio;
  String? _pending;
  String? _error;
  int _historyStart = 0;

  @override
  void dispose() {
    _question.dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _attach(Portfolio? portfolio) {
    if (_portfolio?.id == portfolio?.id) return;
    setState(() {
      _portfolio = portfolio;
      _historyStart = _turns.length;
      _error = null;
    });
  }

  Future<void> _pickPortfolio() async {
    if (!_store.loaded && !_store.loading) _store.load();
    final portfolio = await showModalBottomSheet<Portfolio>(
      context: context,
      showDragHandle: true,
      isScrollControlled: true,
      builder: (context) => SafeArea(
        child: FractionallySizedBox(
          heightFactor: 0.65,
          child: ListenableBuilder(
            listenable: _store,
            builder: (context, _) => ListView(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
              children: [
                Text(
                  'Прикрепить портфель',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 8),
                const Text(
                  'Ассистент получит текущие позиции выбранного портфеля.',
                ),
                const SizedBox(height: 12),
                if (_store.loading) const LinearProgressIndicator(),
                if (_store.error case final error?)
                  ListTile(
                    title: Text(error),
                    subtitle: const Text('Повторить загрузку'),
                    onTap: _store.load,
                    trailing: const Icon(Icons.replay_rounded),
                  ),
                for (final portfolio in _store.portfolios)
                  ListTile(
                    leading: PortfolioAvatar(composite: portfolio.isComposite),
                    title: Text(portfolio.displayName),
                    onTap: () => Navigator.of(context).pop(portfolio),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
    if (mounted && portfolio != null) _attach(portfolio);
  }

  void _scrollToLatest() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _scroll.hasClients) _scroll.jumpTo(0);
    });
  }

  Future<void> _send() async {
    final question = _question.text.trim();
    if (question.isEmpty || _pending != null) return;
    if (question.runes.length > 2000) {
      setState(() => _error = 'Вопрос должен содержать не более 2000 символов');
      return;
    }
    final start = (_turns.length - 4).clamp(_historyStart, _turns.length);
    setState(() {
      _pending = question;
      _error = null;
      _question.clear();
    });
    _scrollToLatest();
    try {
      final json = await _api.post(
        '/api/v1/assistant/chat',
        {
          'question': question,
          if (_portfolio case final portfolio?) 'portfolio_id': portfolio.id,
          'history': [
            for (final turn in _turns.skip(start))
              {
                'question': turn.question,
                'answer': String.fromCharCodes(turn.answer.runes.take(1600)),
              },
          ],
        },
        auth: true,
        timeout: const Duration(seconds: 130),
      ) as Map<String, dynamic>;
      final turn = _Turn(
        question,
        json['answer'] as String,
        (json['sources'] as List)
            .cast<Map<String, dynamic>>()
            .map(FeedPost.fromJson)
            .toList(),
      );
      if (!mounted) return;
      setState(() => _turns.add(turn));
      _scrollToLatest();
    } catch (error) {
      if (!mounted) return;
      _question.text = question;
      setState(() {
        _error = error is ApiException
            ? error.serverMessage ?? error.message
            : 'Не удалось прочитать ответ ассистента';
      });
    } finally {
      if (mounted) setState(() => _pending = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final pending = _pending;
    final portfolio = _portfolio;
    return Scaffold(
      appBar: AppBar(
        title: const Text('ИИ-ассистент'),
        actions: [
          IconButton(
            tooltip: 'Новый диалог',
            onPressed: pending != null || _turns.isEmpty && portfolio == null
                ? null
                : () => setState(() {
                    _turns.clear();
                    _question.clear();
                    _historyStart = 0;
                    _portfolio = null;
                    _error = null;
                  }),
            icon: const Icon(Icons.add_comment_outlined),
          ),
        ],
      ),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 720),
          child: Column(
            children: [
              Expanded(
                child: _turns.isEmpty && pending == null
                    ? ListView(
                        padding: const EdgeInsets.fromLTRB(24, 32, 24, 16),
                        children: [
                          Align(
                            alignment: Alignment.centerLeft,
                            child: CircleAvatar(
                              backgroundColor:
                                  theme.colorScheme.primaryContainer,
                              foregroundColor:
                                  theme.colorScheme.onPrimaryContainer,
                              child: const Icon(Icons.auto_awesome_outlined),
                            ),
                          ),
                          const SizedBox(height: 16),
                          Text(
                            'Что хотите узнать?',
                            style: theme.textTheme.headlineSmall,
                          ),
                          const SizedBox(height: 8),
                          const Text(
                            'Спросите о новостях, прогнозах по бумаге или составе своего портфеля.',
                          ),
                        ],
                      )
                    : ListView.builder(
                        controller: _scroll,
                        reverse: true,
                        padding: const EdgeInsets.all(16),
                        itemCount: _turns.length + (pending == null ? 0 : 1),
                        itemBuilder: (context, index) {
                          if (pending != null && index == 0) {
                            return Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                _QuestionBubble(pending),
                                const SizedBox(height: 12),
                                const LinearProgressIndicator(
                                  semanticsLabel: 'Ассистент готовит ответ',
                                ),
                              ],
                            );
                          }
                          final offset = pending == null ? index : index - 1;
                          final turn = _turns[_turns.length - 1 - offset];
                          return Padding(
                            padding: const EdgeInsets.only(bottom: 20),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                _QuestionBubble(turn.question),
                                const SizedBox(height: 12),
                                SelectableText(turn.answer),
                                if (turn.sources.isNotEmpty)
                                  _Sources(turn.sources),
                              ],
                            ),
                          );
                        },
                      ),
              ),
              SafeArea(
                top: false,
                minimum: const EdgeInsets.fromLTRB(12, 0, 12, 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (_error case final error?)
                      Padding(
                        padding: const EdgeInsets.only(left: 8, top: 8),
                        child: Semantics(
                          liveRegion: true,
                          child: Text(
                            error,
                            style: TextStyle(color: theme.colorScheme.error),
                          ),
                        ),
                      ),
                    if (portfolio == null)
                      TextButton.icon(
                        onPressed: pending == null ? _pickPortfolio : null,
                        icon: const Icon(Icons.attach_file_rounded),
                        label: const Text('Прикрепить портфель'),
                      )
                    else
                      InputChip(
                        avatar: const Icon(Icons.business_center_outlined),
                        label: Text(
                          portfolio.displayName,
                          overflow: TextOverflow.ellipsis,
                        ),
                        deleteButtonTooltipMessage: 'Открепить портфель',
                        onDeleted: pending == null ? () => _attach(null) : null,
                      ),
                    Row(
                      children: [
                        Expanded(
                          child: TextField(
                            controller: _question,
                            enabled: pending == null,
                            minLines: 1,
                            maxLines: 4,
                            maxLength: 2000,
                            decoration: const InputDecoration(
                              labelText: 'Ваш вопрос',
                              border: OutlineInputBorder(),
                              counterText: '',
                            ),
                          ),
                        ),
                        const SizedBox(width: 8),
                        ValueListenableBuilder(
                          valueListenable: _question,
                          builder: (context, value, _) => IconButton.filled(
                            tooltip: 'Отправить вопрос',
                            onPressed:
                                pending == null && value.text.trim().isNotEmpty
                                ? _send
                                : null,
                            icon: const Icon(Icons.arrow_upward_rounded),
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Turn {
  const _Turn(this.question, this.answer, this.sources);

  final String question;
  final String answer;
  final List<FeedPost> sources;
}

class _QuestionBubble extends StatelessWidget {
  const _QuestionBubble(this.question);

  final String question;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Align(
      alignment: Alignment.centerRight,
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: colors.secondaryContainer,
          borderRadius: BorderRadius.circular(18),
        ),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
          child: SelectableText(
            question,
            style: TextStyle(color: colors.onSecondaryContainer),
          ),
        ),
      ),
    );
  }
}

class _Sources extends StatelessWidget {
  const _Sources(this.posts);

  final List<FeedPost> posts;

  @override
  Widget build(BuildContext context) => ExpansionTile(
    tilePadding: EdgeInsets.zero,
    title: Text('Публикации из ленты (${posts.length})'),
    children: [
      for (final post in posts)
        ExpansionTile(
          title: Text(
            '[${post.id}] ${post.title.isEmpty ? post.kind.postLabel : post.title}',
          ),
          subtitle: Text(
            [
              if (post.kind == FeedKind.news)
                post.verified ? 'Проверенная новость' : 'Непроверенная новость'
              else
                post.kind.postLabel,
              MaterialLocalizations.of(context)
                  .formatShortDate(post.createdAt.toLocal()),
              post.author,
              if (post.rating case final rating?)
                'Рейтинг $rating% · оценок ${post.ratingSamples}'
              else
                'Без рейтинга',
            ].join(' · '),
          ),
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
              child: SelectableText(post.body),
            ),
          ],
        ),
    ],
  );
}

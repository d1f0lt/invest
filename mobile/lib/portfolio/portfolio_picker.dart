import 'package:flutter/material.dart';

import 'portfolio_store.dart';
import 'portfolios_screen.dart';

/// Иконка портфеля в цветной плашке — используется в шапке и в списке.
/// [composite] — составной портфель (другая иконка и цвета).
class PortfolioAvatar extends StatelessWidget {
  const PortfolioAvatar({super.key, this.size = 36, this.composite = false});

  final double size;
  final bool composite;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(size * 0.3),
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: composite
              ? const [Color(0xFF1F4E9E), Color(0xFF6A3FB5)]
              : const [Color(0xFF5B2A86), Color(0xFF0E8C8C)],
        ),
      ),
      child: Icon(
        composite ? Icons.layers_rounded : Icons.business_center_rounded,
        size: size * 0.55,
        color: scheme.onPrimary,
      ),
    );
  }
}

/// Шапка слева: иконка + имя текущего портфеля + стрелка вниз. Всё — одна кнопка,
/// открывает экран «Мои портфели».
class PortfolioSelector extends StatelessWidget {
  const PortfolioSelector({super.key});

  @override
  Widget build(BuildContext context) {
    final store = PortfolioStore.instance;
    return ListenableBuilder(
      listenable: store,
      builder: (context, _) {
        final title = store.current?.displayName ?? PortfolioStore.defaultName;
        return InkWell(
          borderRadius: BorderRadius.circular(14),
          onTap: () => openPortfolios(context),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 6),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                PortfolioAvatar(composite: store.current?.isComposite ?? false),
                const SizedBox(width: 10),
                Flexible(
                  child: Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context)
                        .textTheme
                        .titleMedium
                        ?.copyWith(fontWeight: FontWeight.w700),
                  ),
                ),
                const SizedBox(width: 2),
                const Icon(Icons.keyboard_arrow_down_rounded),
              ],
            ),
          ),
        );
      },
    );
  }
}

/// Открывает отдельный экран со списком портфелей.
Future<void> openPortfolios(BuildContext context) {
  final store = PortfolioStore.instance;
  if (!store.loaded && !store.loading) store.load();
  return Navigator.of(context).push(
    MaterialPageRoute<void>(builder: (_) => const PortfoliosScreen()),
  );
}

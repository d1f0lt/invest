import 'package:flutter/material.dart';

import '../portfolio/assets_view.dart';
import '../portfolio/operations_view.dart';
import '../portfolio/portfolio_app_bar.dart';
import '../portfolio/portfolio_store.dart';

/// Вкладка «Портфель»: та же шапка, что на главной, ниже — «Активы» и «Операции».
class PortfolioTab extends StatefulWidget {
  const PortfolioTab({super.key});

  @override
  State<PortfolioTab> createState() => _PortfolioTabState();
}

class _PortfolioTabState extends State<PortfolioTab> {
  @override
  void initState() {
    super.initState();
    final store = PortfolioStore.instance;
    if (!store.loaded && !store.loading) store.load();
  }

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        appBar: portfolioAppBar(
          context,
          upload: true,
          bottom: const TabBar(
            tabs: [Tab(text: 'Активы'), Tab(text: 'Операции')],
          ),
        ),
        body: const TabBarView(
          children: [AssetsView(), OperationsView()],
        ),
      ),
    );
  }
}
